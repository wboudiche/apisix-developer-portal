# Teams / Organizations — Design

**Date:** 2026-06-30
**Status:** Approved, ready for planning
**Surface:** new `internal/teams` package; DB migration (`teams`, `team_members`, `applications.team_id`) + backfill; `internal/auth` (personal-team-on-register); `internal/applications` (team-scoped queries + create-under-team); `internal/subscriptions`/`internal/tryit` (ownership check → membership); `internal/notify` (owner email → team owners); `internal/server` (wiring + routes); `web/` (Teams UI + app-create team selector + app-list team labels).

## Problem

Today every application has a single user owner (`applications.owner_id`), and
every "is this yours?" gate compares it to the logged-in user. Real developers
work in teams and need to **share applications + subscriptions** with colleagues.
This introduces **teams** as the unit of ownership, with a clean migration that
keeps solo use frictionless.

## Locked decisions (from brainstorming)

- **Personal-team unification.** A **team** owns applications; users are
  **members** of teams. Every user has a **personal team** (a team of one), so
  solo use is unchanged. The ownership predicate becomes "is the caller a member
  of the app's team?". Existing apps migrate to their owner's personal team.
- **Roles: `owner` + `member`.** Owner manages membership (add/remove members,
  rename/delete the team) AND has full app access. Member has full access to the
  team's apps + subscriptions (create apps, subscribe, rotate keys, sandbox, set
  OIDC client) but cannot manage membership. The team creator is the sole owner;
  added users are members (owner-transfer / promote deferred).
- **Membership: add an existing registered user by email**, immediate (no accept
  step). Email-invite-and-accept is deferred.
- **`owner_id` is kept as a vestigial `created_by`** (who created the app);
  authorization moves entirely to `team_id` membership.
- Decomposed into **Plan A (backend)** and **Plan B (frontend)**; backend first.

## Data model — migration `0013_teams.sql`

```sql
CREATE TABLE teams (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    personal   BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE team_members (
    team_id    BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('owner','member')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);

ALTER TABLE applications ADD COLUMN team_id BIGINT REFERENCES teams(id);
```

**Backfill (in the same migration, after the DDL):**
1. Create one personal team per existing user:
   `INSERT INTO teams(name, personal) SELECT COALESCE(NULLIF(name,''), email), true FROM users …` —
   but done so each team maps back to its user (insert per user, capturing the
   new team id), e.g. a `DO`/PL block or an `INSERT … RETURNING` loop. Concretely:
   insert a personal team and an `owner` `team_members` row for every user, and
   record the (user_id → personal_team_id) mapping.
2. `UPDATE applications a SET team_id = (the owner's personal team id)` using that
   mapping (`owner_id` → personal team).
3. `ALTER TABLE applications ALTER COLUMN team_id SET NOT NULL;`

(The migration is idempotent-friendly: it runs once on an unmigrated DB. The
plan will spell out the exact SQL — a `DO $$` block that loops over users,
inserts the team + owner membership, and updates that user's apps.)

**On registration (going forward):** in the same transaction that inserts the
user, create their personal team (`personal=true`, name = the user's name or
email) and an `owner` `team_members` row.

## Package `internal/teams`

Repo + Service + Handler (mirrors `internal/applications`).

**Repo (over the pool):**
- `Create(ctx, name string, ownerUserID int64) (Team, error)` — inserts a
  non-personal team + an `owner` membership, in one tx.
- `CreatePersonal(ctx, tx, userID int64, name string) (int64, error)` — used by
  registration (personal team + owner membership).
- `ListForUser(ctx, userID int64) ([]TeamSummary, error)` — teams the user
  belongs to, each with `role` + `memberCount`.
- `Get(ctx, teamID int64) (Team, error)`; `Members(ctx, teamID int64)
  ([]Member, error)` (`userID, email, name, role`).
- `Role(ctx, teamID, userID int64) (string, bool, error)` — the user's role +
  whether they're a member.
- `PersonalTeamID(ctx, userID int64) (int64, error)` — the user's personal team
  (used when `POST /api/applications` omits `teamId`).
- `IsMemberOfApp(ctx, userID, appID int64) (bool, error)` — joins
  `applications` → `team_members`; the new ownership predicate.
- `AddMemberByEmail(ctx, teamID int64, email string) error` — looks up the user
  by email (→ `ErrUserNotFound`), inserts a `member` row (→ `ErrAlreadyMember`
  on conflict); rejects `personal` teams (→ `ErrPersonalTeam`).
- `RemoveMember(ctx, teamID, userID int64) error` — deletes the membership;
  rejects removing the last `owner` (→ `ErrLastOwner`) and `personal` teams.
- `Rename(ctx, teamID int64, name string) error`; `Delete(ctx, teamID int64)
  error` — `Delete` rejects when the team has applications (→ `ErrTeamHasApps`)
  or is personal.
- `OwnerEmailsForApp(ctx, appID int64) ([]string, string, error)` — the team
  owners' emails + app name, for the notify integration (replaces the single
  `OwnerEmailForApp`).

**Service** wraps the repo with the authorization rules (owner-only for
management) and maps errors to HTTP statuses via the handler.

**Handler routes (all behind `requireAuth`):**
- `GET /api/teams` → `ListForUser`.
- `POST /api/teams` `{name}` → `Create` (caller = owner). 400 on empty name.
- `GET /api/teams/{id}/members` → `Members`; 403 unless the caller is a member.
- `POST /api/teams/{id}/members` `{email}` → owner-only; 404 `ErrUserNotFound`,
  409 `ErrAlreadyMember`/`ErrPersonalTeam`.
- `DELETE /api/teams/{id}/members/{userId}` → owner-only; 409 `ErrLastOwner`,
  422/409 `ErrPersonalTeam`.
- `PATCH /api/teams/{id}` `{name}` → owner-only rename (non-personal).
- `DELETE /api/teams/{id}` → owner-only; 409 `ErrTeamHasApps`, 409
  `ErrPersonalTeam`.

## Applications become team-scoped

- **`GET /api/applications`** — apps across **all** the caller's teams. The list
  query's `WHERE a.owner_id=$1` → `WHERE a.team_id IN (SELECT team_id FROM
  team_members WHERE user_id=$1)`. Each row now carries `team_id` + `team_name`
  (join `teams`). The `Application` view + `AppDetail` gain `teamId`/`teamName`.
- **`POST /api/applications`** `{name, description, teamId}` — `teamId` must be a
  team the caller belongs to (else 403/400); **defaults to the caller's personal
  team** when omitted. Inserts `team_id = teamId`, `owner_id = caller` (now
  "created_by").
- **Every per-app gate** — app detail, subscribe/unsubscribe, key rotation,
  sandbox enable/rotate, OIDC client, try-it — switches from the `owner_id`
  comparison to **`teams.IsMemberOfApp(caller, appID)`**. The `OwnerCheck` /
  `OwnsApp` adapter in `server.go`/`tryit_adapters.go` is rewired to call it.
- **`notify`** — `Notifier.deliver` resolves the recipient for *Approved* /
  *Rejected* via `teams.OwnerEmailsForApp` (the team's owner(s)), not a single
  `owner_id`. `internal/notify/repo.go`'s `OwnerEmailForApp` is replaced/augmented
  accordingly (it may simply delegate to the teams query). Admin emails for
  *Requested* are unchanged.

## Frontend (Plan B)

- **Teams area** (a nav link "Équipes"): list my teams (name, my role, member
  count); **create a team**; a **team detail** view that lists members and — for
  owners — **adds a member by email**, **removes** members, **renames**, and
  **deletes** the team (delete disabled/blocked when the team has apps).
- **Applications page**: lists apps from all my teams, each with a **team label**;
  the **create-app modal** gains a **team selector** (defaults to my personal
  team). `AppDetail` shows the owning team.
- **Types/client:** `Team` (`id, name, personal, role, memberCount`),
  `TeamMember` (`userId, email, name, role`); `getTeams`/`createTeam`/
  `getTeamMembers`/`addTeamMember`/`removeTeamMember`/`renameTeam`/`deleteTeam`;
  `Application`/`AppDetail` gain `teamId`/`teamName`; `createApplication` gains an
  optional `teamId`. French copy, Atlas tokens.

## Testing

### Backend (Go)

- **Migration backfill:** after migrate, every existing user has exactly one
  `personal` team with an `owner` membership; every pre-existing app has a
  `team_id` equal to its owner's personal team; `applications.team_id` is
  `NOT NULL`.
- **teams repo:** `Create` makes the team + owner membership; `ListForUser`
  returns the right teams with role + member count; `AddMemberByEmail` adds a
  member (404 unknown email, 409 duplicate, rejects personal); `RemoveMember`
  removes (rejects last owner + personal); `IsMemberOfApp` true for a teammate,
  false for a non-member; `Delete` rejects when the team has apps; `Rename`
  works; `OwnerEmailsForApp` returns the owners.
- **Registration:** registering a user creates their personal team + owner
  membership in the same tx.
- **Applications:** the list returns apps from all the caller's teams with
  `teamId`/`teamName`; create-under-team uses the given team (default personal);
  creating under a team the caller doesn't belong to is rejected.
- **Authorization:** a team member can fetch/act on a teammate's app; a
  non-member gets 403 on detail/subscribe/rotate; team management endpoints are
  owner-only (a member gets 403).
- **notify:** *Approved*/*Rejected* resolve the team owner(s)' email via the new
  query; existing notify tests still pass.

### Frontend (vitest)

- Teams list renders my teams + roles; create-team posts and refreshes.
- Member add/remove (owner) calls the right endpoints; a member sees a read-only
  member list (no add/remove controls).
- The app-create modal's team selector defaults to the personal team and sends
  `teamId`; the app list shows each app's team label.

### Live (controller)

Register two users A and B. A creates a team "Acme" and adds B by email. B's
`GET /api/teams` shows Acme; B opens an app A created under Acme and can
subscribe + rotate its key; A (owner) removes B → B now gets 403 on that app.
Create an app under Acme (non-personal) and confirm its `teamId`. Confirm the
migration mapped a pre-existing app to its owner's personal team. **Look at the
UI.**

## Out of scope (deferred)

- Owner transfer / promote-to-owner / multiple-owner management.
- Email-invite-and-accept for not-yet-registered users (V1 adds existing users).
- Per-app (vs per-team) sharing; nested teams; org-level RBAC beyond owner/member.
- Team-level billing / quotas.
- Renaming or deleting the personal team (it's fixed, always present).
```
