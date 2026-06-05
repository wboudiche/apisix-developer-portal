# Application Detail Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Applications page with a per-application detail page (`/applications/:id`) matching the user's `/application.html` blueprint — real data where the backend has it, blueprint demo values elsewhere (isolated in `demo.ts`).

**Architecture:** Bottom-up: tokens → scoped CSS → page primitives (helpers/demo/Toast/ConfirmModal) → tab components (pure, prop-driven, individually tested) → AppSwitcher → the AppDetailPage shell + routing, then delete the old page. The shell owns all data fetching and mutations; tabs are presentational.

**Tech Stack:** React 19 + TS, react-router-dom v7, Vitest + RTL (jsdom), plain CSS with oklch tokens.

**Spec:** `docs/superpowers/specs/2026-06-05-application-detail-page-design.md`
**Blueprint (visual source of truth):** `/application.html` at repo root.

**CRITICAL traps (do not "fix" these):**
1. CSS is bundled globally. catalog.css owns `.card{opacity:0}` (invisible-element trap), `.pill`, `.tag`; base.css owns `.modal`. The blueprint classes are RENAMED: `.card`→`.dcard`, `.pill`→`.stpill`, `.modal`→`.dmodal`, `.tag`→`.envtag`; keyframes `fade`→`ad-fade`, `pop`→`ad-pop`. All rules scoped under `.appdetail`.
2. IDE/gopls diagnostics in this repo are often stale — trust `npx vitest run` output only.
3. `getPlans()` takes NO token. `Application` HAS `createdAt` and `description` (both real).
4. Existing tests stub `window.matchMedia` in `web/src/setupTests.ts`; clipboard must be stubbed per-test.

**API shapes used throughout (from `web/src/api/types.ts` / `client.ts`):**
`Application{id,ownerId,name,description,createdAt}` · `AppDetail{apiKey,consumerUsername,subscriptions:SubscriptionView[]}` · `SubscriptionView{productId,productName,version,contextPath,planId,planName,status}` · `Plan{id,name,rateLimit,windowSeconds}` · `getApplications(token)` · `getApplicationDetail(token,appId)` · `createApplication(token,name,description)` · `unsubscribe(token,appId,productId)` · `getPlans()`.

---

### Task 1: Soft status tokens

**Files:**
- Modify: `web/src/styles/tokens.css`

- [ ] **Step 1: Add light values**

In `:root{...}`, immediately after the line `--danger: oklch(58% 0.20 25);` add:

```css
  --success-soft: oklch(95% 0.04 155);
  --warn-soft: oklch(96% 0.05 85);
  --danger-soft: oklch(96% 0.04 25);
```

- [ ] **Step 2: Add dark values**

In `:root[data-theme="dark"]{...}`, immediately after the line `--danger: oklch(66% 0.19 25);` add:

```css
  --success-soft: oklch(32% 0.06 155);
  --warn-soft: oklch(33% 0.07 85);
  --danger-soft: oklch(32% 0.07 25);
```

- [ ] **Step 3: Sanity check**

Run: `cd web && npx vitest run src/components/AuthShell.test.tsx`
Expected: 3 passed (CSS still parses).

- [ ] **Step 4: Commit**

```bash
git add web/src/styles/tokens.css
git commit -m "feat(web): soft status tokens for app detail page"
```

---

### Task 2: `appdetail.css` (scoped blueprint port)

**Files:**
- Create: `web/src/styles/appdetail.css`

- [ ] **Step 1: Collision grep**

Run:
```bash
cd web && grep -nE '\.(appdetail|crumbs|apphead|glyph|htext|stpill|dcard|btn|tabs|panel|section-title|stats|stat|keygrid|keycard|keyrow|iconbtn|keymeta|code|cbar|tbl|tblwrap|apicell|plan-pill|rate|bar|rowsub|rowact|linkbtn|chart|twocol|feed|field|danger-zone|switch|trigger|menu|toast|scrim|dmodal|envtag|mono)\b' src/styles/catalog.css src/styles/base.css
```
Expected hits to IGNORE (they're fine): none should be unscoped duplicates of our names. If `.btn`, `.tabs`, `.stats`, `.stat`, `.field`, `.feed`, `.toast`, `.switch`, `.menu`, `.code`, `.bar`, `.rate`, `.mono` appear in catalog.css or base.css as GLOBAL rules (not under another ancestor), rename the colliding class in the CSS below AND remember the rename for the component tasks — report it in your completion summary. (`.field` in auth.css is scoped under `.auth-shell` — no conflict.)

- [ ] **Step 2: Write `web/src/styles/appdetail.css`**

This is the blueprint `<style>` block (lines ~98–311 of `/application.html`) with: every rule prefixed `.appdetail `, the rename table applied, the topbar section dropped (the app keeps its existing `TopBar`), `:root` dropped (tokens exist), and four hardcoded light-grays replaced with `color-mix` so dark mode works. Exact content:

```css
/* Application detail page — port of /application.html blueprint.
   Scoped under .appdetail. Renames (global-CSS collision proofing):
   .card→.dcard  .pill→.stpill  .modal→.dmodal  .tag→.envtag
   keyframes: fade→ad-fade  pop→ad-pop
   (catalog.css owns global .card{opacity:0}, .pill, .tag; base.css owns .modal) */

.appdetail{max-width:1120px;margin:0 auto;padding:26px 28px 80px}
.appdetail .mono{font-family:var(--font-mono);font-variant-numeric:tabular-nums}

.appdetail .crumbs{display:flex;align-items:center;gap:8px;font-size:13px;color:var(--muted);margin-bottom:18px}
.appdetail .crumbs a:hover{color:var(--fg)}
.appdetail .crumbs svg{width:14px;height:14px;color:var(--faint)}

/* app header */
.appdetail .apphead{display:flex;align-items:flex-start;gap:18px;flex-wrap:wrap}
.appdetail .apphead .glyph{
  width:56px;height:56px;border-radius:15px;flex:none;display:grid;place-items:center;
  color:#fff;font-family:var(--font-display);font-weight:700;font-size:22px;box-shadow:var(--shadow);
}
.appdetail .apphead .htext{min-width:0}
.appdetail .apphead h1{font-family:var(--font-display);font-size:26px;font-weight:700;letter-spacing:-.02em;display:flex;align-items:center;gap:12px;flex-wrap:wrap}
.appdetail .apphead .meta{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin-top:7px;font-size:13px;color:var(--muted)}
.appdetail .apphead .meta .mono{color:var(--fg)}
.appdetail .apphead .meta .sep{width:3px;height:3px;border-radius:50%;background:var(--border-2)}
.appdetail .apphead .actions{margin-left:auto;display:flex;gap:10px;align-self:center}

.appdetail .stpill{display:inline-flex;align-items:center;gap:6px;font-size:12px;font-weight:600;padding:4px 11px;border-radius:999px;letter-spacing:.01em}
.appdetail .stpill .led{width:6px;height:6px;border-radius:50%}
.appdetail .stpill.ok{background:var(--success-soft);color:oklch(42% 0.12 155)}
.appdetail .stpill.ok .led{background:var(--success);box-shadow:0 0 0 3px oklch(58% 0.13 155 /.18)}
.appdetail .stpill.warn{background:var(--warn-soft);color:oklch(48% 0.12 75)}
.appdetail .stpill.warn .led{background:var(--warn)}
.appdetail .stpill.muted{background:color-mix(in oklab, var(--fg) 7%, transparent);color:var(--muted)}
.appdetail .stpill.muted .led{background:var(--faint)}

.appdetail .btn{height:40px;padding:0 16px;border-radius:11px;font-size:14px;font-weight:600;display:inline-flex;align-items:center;gap:8px;transition:.16s;border:1px solid transparent}
.appdetail .btn svg{width:16px;height:16px}
.appdetail .btn.primary{background:linear-gradient(150deg,var(--accent),var(--accent-d));color:#fff;box-shadow:0 5px 16px oklch(55% 0.205 27 /.3)}
.appdetail .btn.primary:hover{transform:translateY(-1px);box-shadow:0 9px 24px oklch(55% 0.205 27 /.4)}
.appdetail .btn.ghost{background:var(--surface);border-color:var(--border-2);color:var(--fg)}
.appdetail .btn.ghost:hover{border-color:var(--accent);color:var(--accent);background:var(--accent-soft)}
.appdetail .btn.danger{background:var(--surface);border-color:var(--border-2);color:var(--danger)}
.appdetail .btn.danger:hover{background:var(--danger-soft);border-color:var(--danger)}
.appdetail .btn.sm{height:34px;padding:0 13px;font-size:13px;border-radius:9px}

/* tabs */
.appdetail .tabs{display:flex;gap:4px;margin:26px 0 24px;border-bottom:1px solid var(--border);overflow-x:auto}
.appdetail .tabs button{
  background:none;border:0;padding:11px 4px;margin-right:20px;font-size:14.5px;font-weight:500;color:var(--muted);
  position:relative;white-space:nowrap;transition:.15s;
}
.appdetail .tabs button:hover{color:var(--fg)}
.appdetail .tabs button.on{color:var(--accent);font-weight:600}
.appdetail .tabs button.on::after{content:"";position:absolute;left:0;right:0;bottom:-1px;height:2px;background:var(--accent);border-radius:2px}
.appdetail .tabs button .badge{margin-left:7px;font-family:var(--font-mono);font-size:11px;color:var(--faint);border:1px solid var(--border-2);border-radius:6px;padding:1px 6px}
.appdetail .tabs button.on .badge{color:var(--accent);border-color:var(--accent-soft);background:var(--accent-soft)}

.appdetail .panel{animation:ad-fade .25s ease}
@keyframes ad-fade{from{opacity:0;transform:translateY(4px)}to{opacity:1;transform:none}}

/* cards */
.appdetail .dcard{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);box-shadow:var(--shadow)}
.appdetail .dcard .ch{display:flex;align-items:center;gap:12px;padding:18px 20px;border-bottom:1px solid var(--border)}
.appdetail .dcard .ch h3{font-family:var(--font-display);font-size:16px;font-weight:600;letter-spacing:-.01em}
.appdetail .dcard .ch p{font-size:12.5px;color:var(--muted);margin-top:2px}
.appdetail .dcard .ch .right{margin-left:auto}
.appdetail .dcard .cb{padding:20px}

.appdetail .section-title{font-family:var(--font-display);font-size:13px;font-weight:600;text-transform:uppercase;letter-spacing:.08em;color:var(--muted);margin:0 0 14px}

/* stat grid */
.appdetail .stats{display:grid;grid-template-columns:repeat(4,1fr);gap:14px;margin-bottom:24px}
.appdetail .stat{background:var(--surface);border:1px solid var(--border);border-radius:var(--r);padding:17px 18px}
.appdetail .stat .k{font-size:12px;color:var(--muted);display:flex;align-items:center;gap:7px}
.appdetail .stat .k svg{width:15px;height:15px;color:var(--faint)}
.appdetail .stat .v{font-family:var(--font-display);font-size:27px;font-weight:700;letter-spacing:-.02em;margin-top:9px;font-variant-numeric:tabular-nums}
.appdetail .stat .v small{font-size:15px;color:var(--muted);font-weight:500}
.appdetail .stat .d{font-size:12px;margin-top:4px;display:flex;align-items:center;gap:5px}
.appdetail .stat .d.up{color:var(--success)}.appdetail .stat .d.down{color:var(--danger)}.appdetail .stat .d.flat{color:var(--muted)}
.appdetail .stat .d svg{width:13px;height:13px}

/* key cards */
.appdetail .keygrid{display:grid;grid-template-columns:1fr 1fr;gap:16px}
.appdetail .keycard{border:1px solid var(--border);border-radius:var(--r);background:var(--surface);overflow:hidden}
.appdetail .keycard.prod{border-color:oklch(80% 0.07 27)}
.appdetail .keycard .kh{display:flex;align-items:center;gap:10px;padding:15px 18px;border-bottom:1px solid var(--border)}
.appdetail .keycard .kh .env{font-family:var(--font-display);font-weight:600;font-size:15px;display:flex;align-items:center;gap:9px}
.appdetail .keycard .kh .env .envtag{font-family:var(--font-mono);font-size:10.5px;text-transform:uppercase;letter-spacing:.08em;padding:3px 8px;border-radius:6px}
.appdetail .keycard.prod .kh .env .envtag{background:var(--accent-soft);color:var(--accent)}
.appdetail .keycard.sbx .kh .env .envtag{background:color-mix(in oklab, var(--fg) 7%, transparent);color:var(--muted)}
.appdetail .keycard .kb{padding:18px}
.appdetail .keyrow{display:flex;align-items:center;gap:10px;background:var(--bg);border:1px solid var(--border-2);border-radius:10px;padding:11px 13px}
.appdetail .keyrow code{flex:1;font-family:var(--font-mono);font-size:13.5px;letter-spacing:.02em;color:var(--fg);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.appdetail .iconbtn{width:32px;height:32px;border-radius:8px;border:1px solid var(--border-2);background:var(--surface);display:grid;place-items:center;color:var(--muted);flex:none;transition:.14s}
.appdetail .iconbtn:hover{color:var(--accent);border-color:var(--accent);background:var(--accent-soft)}
.appdetail .iconbtn.copied{color:var(--success);border-color:var(--success)}
.appdetail .iconbtn svg{width:16px;height:16px}
.appdetail .keymeta{display:flex;align-items:center;justify-content:space-between;margin-top:14px;font-size:12.5px;color:var(--muted)}
.appdetail .keymeta .rotate{color:var(--accent);font-weight:600;display:inline-flex;align-items:center;gap:6px;background:none;border:0;font-size:12.5px}
.appdetail .keymeta .rotate:hover{text-decoration:underline}
.appdetail .keymeta .rotate svg{width:14px;height:14px}

/* code block */
.appdetail .code{background:var(--ink);border-radius:12px;overflow:hidden;margin-top:4px}
.appdetail .code .cbar{display:flex;align-items:center;gap:8px;padding:10px 14px;border-bottom:1px solid oklch(100% 0 0 /.08)}
.appdetail .code .cbar i{width:10px;height:10px;border-radius:50%;display:block}
.appdetail .code .cbar i:nth-child(1){background:oklch(70% 0.18 25)}
.appdetail .code .cbar i:nth-child(2){background:oklch(82% 0.14 85)}
.appdetail .code .cbar i:nth-child(3){background:oklch(75% 0.16 150)}
.appdetail .code .cbar span{margin-left:6px;font-family:var(--font-mono);font-size:11px;color:oklch(78% 0.01 262)}
.appdetail .code .cbar .copy{margin-left:auto;font-family:var(--font-mono);font-size:11px;color:oklch(82% 0.01 262);border:1px solid oklch(100% 0 0 /.14);border-radius:6px;padding:3px 9px;background:transparent}
.appdetail .code .cbar .copy:hover{background:oklch(100% 0 0 /.08);color:#fff}
.appdetail .code pre{padding:15px 16px;font-family:var(--font-mono);font-size:13px;line-height:1.8;color:oklch(90% 0.01 262);overflow-x:auto}
.appdetail .code .c{color:oklch(62% 0.02 262)}.appdetail .code .cmd{color:oklch(80% 0.14 150)}
.appdetail .code .flag{color:oklch(82% 0.10 32)}.appdetail .code .str{color:oklch(80% 0.10 85)}

/* table */
.appdetail .tbl{width:100%;border-collapse:collapse}
.appdetail .tbl th{text-align:left;font-size:11.5px;font-weight:600;text-transform:uppercase;letter-spacing:.06em;color:var(--muted);padding:0 16px 12px;border-bottom:1px solid var(--border)}
.appdetail .tbl td{padding:15px 16px;border-bottom:1px solid var(--border);font-size:14px;vertical-align:middle}
.appdetail .tbl tr:last-child td{border-bottom:0}
.appdetail .tbl tbody tr{transition:.12s}
.appdetail .tbl tbody tr:hover{background:color-mix(in oklab, var(--fg) 3%, transparent)}
.appdetail .apicell{display:flex;align-items:center;gap:12px}
.appdetail .apicell .ig{width:36px;height:36px;border-radius:9px;flex:none;display:grid;place-items:center;color:#fff;font-family:var(--font-mono);font-weight:600;font-size:12px}
.appdetail .apicell .nm{font-weight:600;font-size:14px}
.appdetail .apicell .cx{font-family:var(--font-mono);font-size:11.5px;color:var(--muted);display:block}
.appdetail .plan-pill{display:inline-flex;align-items:center;gap:7px;font-size:13px;font-weight:600}
.appdetail .plan-pill .dot{width:8px;height:8px;border-radius:50%}
.appdetail .rate{font-family:var(--font-mono);font-size:13px;color:var(--muted)}
.appdetail .bar{height:6px;border-radius:3px;background:color-mix(in oklab, var(--fg) 8%, transparent);overflow:hidden;width:110px}
.appdetail .bar i{display:block;height:100%;border-radius:3px;background:linear-gradient(90deg,var(--accent),var(--accent-d))}
.appdetail .bar.hi i{background:linear-gradient(90deg,var(--warn),oklch(66% 0.17 55))}
.appdetail .rowsub{font-size:11.5px;color:var(--faint);margin-top:3px;font-family:var(--font-mono)}
.appdetail .rowact{display:flex;gap:8px;justify-content:flex-end}
.appdetail .linkbtn{font-size:13px;color:var(--muted);font-weight:500;cursor:pointer}
.appdetail .linkbtn:hover{color:var(--accent)}
.appdetail .linkbtn.danger:hover{color:var(--danger)}

/* usage chart */
.appdetail .chart{display:flex;align-items:flex-end;gap:10px;height:200px;padding:10px 4px 0}
.appdetail .chart .col{flex:1;display:flex;flex-direction:column;align-items:center;gap:8px;height:100%;justify-content:flex-end}
.appdetail .chart .col .bw{width:100%;max-width:34px;border-radius:7px 7px 0 0;background:linear-gradient(180deg,var(--accent),oklch(62% 0.16 27));transition:.5s cubic-bezier(.2,.8,.2,1);position:relative}
.appdetail .chart .col:hover .bw{filter:brightness(1.08)}
.appdetail .chart .col .bw::after{content:attr(data-v);position:absolute;top:-22px;left:50%;transform:translateX(-50%);font-family:var(--font-mono);font-size:11px;color:var(--muted);opacity:0;transition:.15s;white-space:nowrap}
.appdetail .chart .col:hover .bw::after{opacity:1}
.appdetail .chart .col small{font-family:var(--font-mono);font-size:11px;color:var(--faint)}

.appdetail .twocol{display:grid;grid-template-columns:1.4fr 1fr;gap:20px;align-items:start}

.appdetail .feed{list-style:none}
.appdetail .feed li{display:flex;gap:13px;padding:12px 0;border-bottom:1px solid var(--border);font-size:13.5px}
.appdetail .feed li:last-child{border-bottom:0}
.appdetail .feed .fi{width:30px;height:30px;border-radius:8px;flex:none;display:grid;place-items:center;background:var(--bg);border:1px solid var(--border)}
.appdetail .feed .fi svg{width:15px;height:15px;color:var(--muted)}
.appdetail .feed .ft{flex:1}
.appdetail .feed .ft b{font-weight:600}
.appdetail .feed .ft small{display:block;color:var(--faint);font-size:11.5px;margin-top:2px;font-family:var(--font-mono)}

/* form (settings) */
.appdetail .field{margin-bottom:18px;max-width:480px}
.appdetail .field label{display:block;font-size:13px;font-weight:600;margin-bottom:7px}
.appdetail .field input,.appdetail .field textarea,.appdetail .field select{
  width:100%;border:1px solid var(--border-2);border-radius:11px;background:var(--bg);
  padding:11px 13px;font-size:14px;font-family:inherit;color:var(--fg);transition:.16s;
}
.appdetail .field textarea{resize:vertical;min-height:80px;line-height:1.5}
.appdetail .field input:focus,.appdetail .field textarea:focus,.appdetail .field select:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 4px var(--accent-soft);background:var(--surface)}
.appdetail .field .hint{font-size:12px;color:var(--muted);margin-top:6px}
.appdetail .danger-zone{border:1px solid var(--danger-soft);background:var(--danger-soft);border-radius:var(--r);padding:18px 20px;margin-top:8px;display:flex;align-items:center;gap:16px;flex-wrap:wrap}
.appdetail .danger-zone .dz-t{flex:1;min-width:220px}
.appdetail .danger-zone h4{font-size:14px;font-weight:600;color:oklch(45% 0.16 25)}
.appdetail .danger-zone p{font-size:13px;color:oklch(50% 0.08 25);margin-top:3px}

/* dropdown (app switcher) */
.appdetail .switch{position:relative}
.appdetail .switch .trigger{display:inline-flex;align-items:center;gap:9px;height:40px;padding:0 13px;border:1px solid var(--border-2);border-radius:11px;background:var(--surface);font-size:14px;font-weight:600;color:var(--fg)}
.appdetail .switch .trigger:hover{border-color:var(--accent)}
.appdetail .switch .trigger svg{width:15px;height:15px;color:var(--muted)}
.appdetail .switch .menu{position:absolute;top:46px;left:0;min-width:260px;background:var(--surface);border:1px solid var(--border);border-radius:13px;box-shadow:var(--shadow-h);padding:7px;z-index:30;display:none}
.appdetail .switch.open .menu{display:block}
.appdetail .switch .menu a{display:flex;align-items:center;gap:11px;padding:9px 11px;border-radius:9px;font-size:14px;cursor:pointer}
.appdetail .switch .menu a:hover{background:color-mix(in oklab, var(--fg) 4%, transparent)}
.appdetail .switch .menu a.cur{background:var(--accent-soft)}
.appdetail .switch .menu a .mg{width:30px;height:30px;border-radius:8px;flex:none;display:grid;place-items:center;color:#fff;font-family:var(--font-display);font-weight:700;font-size:13px}
.appdetail .switch .menu a .mt small{display:block;color:var(--muted);font-size:11.5px;font-family:var(--font-mono)}
.appdetail .switch .menu .div{height:1px;background:var(--border);margin:6px 4px}
.appdetail .switch .menu .new{color:var(--accent);font-weight:600}

/* toast */
.appdetail-toast{position:fixed;bottom:24px;left:50%;transform:translateX(-50%) translateY(20px);background:var(--ink);color:#fff;font-size:13.5px;font-weight:500;padding:12px 18px;border-radius:11px;box-shadow:var(--shadow-h);display:flex;align-items:center;gap:9px;opacity:0;pointer-events:none;transition:.25s;z-index:90}
.appdetail-toast.show{opacity:1;transform:translateX(-50%) translateY(0)}
.appdetail-toast svg{width:17px;height:17px;color:var(--success)}

/* modal */
.appdetail-scrim{position:fixed;inset:0;background:oklch(20% 0.02 262 /.45);backdrop-filter:blur(3px);display:grid;place-items:center;z-index:80;padding:20px}
.appdetail-scrim .dmodal{background:var(--surface);border-radius:18px;box-shadow:var(--shadow-h);max-width:440px;width:100%;padding:26px;animation:ad-pop .2s ease}
@keyframes ad-pop{from{opacity:0;transform:scale(.96)}to{opacity:1;transform:none}}
.appdetail-scrim .dmodal .mi{width:46px;height:46px;border-radius:12px;display:grid;place-items:center;margin-bottom:15px}
.appdetail-scrim .dmodal .mi svg{width:24px;height:24px}
.appdetail-scrim .dmodal h3{font-family:var(--font-display);font-size:19px;font-weight:700;letter-spacing:-.01em}
.appdetail-scrim .dmodal p{font-size:14px;color:var(--muted);line-height:1.55;margin-top:9px}
.appdetail-scrim .dmodal .ma{display:flex;gap:10px;margin-top:22px;justify-content:flex-end}
.appdetail-scrim .dmodal .field{margin:16px 0 0;max-width:none}
.appdetail-scrim .dmodal .field input{width:100%;border:1px solid var(--border-2);border-radius:11px;background:var(--bg);padding:11px 13px;font-size:14px;font-family:inherit;color:var(--fg)}
.appdetail-scrim .dmodal .btn{height:40px;padding:0 16px;border-radius:11px;font-size:14px;font-weight:600;display:inline-flex;align-items:center;gap:8px;border:1px solid transparent}
.appdetail-scrim .dmodal .btn.primary{background:linear-gradient(150deg,var(--accent),var(--accent-d));color:#fff}
.appdetail-scrim .dmodal .btn.ghost{background:var(--surface);border-color:var(--border-2);color:var(--fg)}
.appdetail-scrim .dmodal .btn.danger{background:var(--surface);border-color:var(--border-2);color:var(--danger)}
.appdetail-scrim .dmodal .btn.danger:hover{background:var(--danger-soft);border-color:var(--danger)}

/* dark-mode readability for blueprint literals */
:root[data-theme="dark"] .appdetail .stpill.ok{color:oklch(80% 0.1 155)}
:root[data-theme="dark"] .appdetail .stpill.warn{color:oklch(85% 0.1 85)}
:root[data-theme="dark"] .appdetail .danger-zone h4{color:oklch(78% 0.12 25)}
:root[data-theme="dark"] .appdetail .danger-zone p{color:oklch(70% 0.06 25)}
:root[data-theme="dark"] .appdetail .keycard.prod{border-color:oklch(45% 0.09 27)}

/* responsive (blueprint, minus topbar rules) */
@media(max-width:880px){
  .appdetail .stats{grid-template-columns:1fr 1fr}
  .appdetail .keygrid{grid-template-columns:1fr}
  .appdetail .twocol{grid-template-columns:1fr}
}
@media(max-width:560px){
  .appdetail .stats{grid-template-columns:1fr}
  .appdetail .apphead .actions{margin-left:0;width:100%}
  .appdetail .apphead .actions .btn{flex:1;justify-content:center}
  .appdetail .tblwrap{overflow-x:auto}
  .appdetail .tbl{min-width:620px}
}
```

Notes (intentional differences from blueprint — do not "fix"):
- Toast/scrim use page-unique top-level classes `.appdetail-toast`/`.appdetail-scrim` because they render as fixed overlays (portal-like) and must not depend on an `.appdetail` ancestor.
- `.panel{display:none}`/`.panel.on` dropped — React renders only the active tab; `.panel` keeps just the fade animation.
- `.keymeta .rotate` gains `background:none;border:0;font-size:12.5px` because it's a `<button>` in React (blueprint had button styling resets globally).
- `.stat .v small` replaces blueprint's inline `style="font-size:15px…"` units.
- `.apicell .cx` gains `display:block` (blueprint relied on span flow inside a wrapping span).
- Light-gray literals → `color-mix(...)` for dark mode; dark-mode override block at the end.

- [ ] **Step 3: Commit**

```bash
git add web/src/styles/appdetail.css
git commit -m "feat(web): scoped appdetail.css ported from application.html blueprint"
```

---

### Task 3: Page primitives — `helpers.ts`, `demo.ts`, `Toast`, `ConfirmModal`

**Files:**
- Create: `web/src/pages/application/helpers.ts`
- Create: `web/src/pages/application/demo.ts`
- Create: `web/src/pages/application/Toast.tsx`
- Create: `web/src/pages/application/ConfirmModal.tsx`
- Test: `web/src/pages/application/helpers.test.ts`
- Test: `web/src/pages/application/ConfirmModal.test.tsx`

- [ ] **Step 1: Write failing tests**

`web/src/pages/application/helpers.test.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { appRef, initials, maskKey, rateLabel, statusPill, frDate } from './helpers'

describe('helpers', () => {
  it('formats the app reference', () => expect(appRef(7)).toBe('app_7'))
  it('derives initials from one or two words', () => {
    expect(initials('Boutique Mobile')).toBe('BM')
    expect(initials('analytics')).toBe('A')
    expect(initials('  ')).toBe('?')
  })
  it('masks a key keeping first 8 and last 2 chars', () => {
    expect(maskKey('ax_live_a3f9c1e7b240')).toBe('ax_live_' + '•'.repeat(10) + '40')
    expect(maskKey('short')).toBe('short')
  })
  it('labels plan rates, minute window as /min', () => {
    expect(rateLabel({ id: 1, name: 'Gold', rateLimit: 1000, windowSeconds: 60 })).toBe('1 000 / min')
    expect(rateLabel({ id: 2, name: 'X', rateLimit: 50, windowSeconds: 10 })).toBe('50 / 10s')
    expect(rateLabel(undefined)).toBe('—')
  })
  it('maps subscription status to pill class/label', () => {
    expect(statusPill('active')).toEqual({ cls: 'ok', label: 'Active' })
    expect(statusPill('pending')).toEqual({ cls: 'warn', label: 'En attente' })
    expect(statusPill('rejected')).toEqual({ cls: 'muted', label: 'Rejeté' })
  })
  it('formats dates in french and tolerates garbage', () => {
    expect(frDate('2026-03-12T10:00:00Z')).toMatch(/mars/)
    expect(frDate('nope')).toBe('—')
  })
})
```

`web/src/pages/application/ConfirmModal.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ConfirmModal } from './ConfirmModal'

describe('ConfirmModal', () => {
  it('renders nothing when spec is null', () => {
    const { container } = render(<ConfirmModal spec={null} onClose={() => {}} />)
    expect(container.firstChild).toBeNull()
  })
  it('confirms then closes', async () => {
    const onConfirm = vi.fn(); const onClose = vi.fn()
    render(<ConfirmModal spec={{ title: 'Résilier ?', body: 'corps', confirmLabel: 'Résilier', danger: true, onConfirm }} onClose={onClose} />)
    expect(screen.getByText('Résilier ?')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Résilier' }))
    expect(onConfirm).toHaveBeenCalledOnce()
    expect(onClose).toHaveBeenCalledOnce()
  })
  it('cancels without confirming', async () => {
    const onConfirm = vi.fn(); const onClose = vi.fn()
    render(<ConfirmModal spec={{ title: 't', body: 'b', onConfirm }} onClose={onClose} />)
    await userEvent.click(screen.getByRole('button', { name: 'Annuler' }))
    expect(onClose).toHaveBeenCalledOnce()
    expect(onConfirm).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/application/`
Expected: FAIL — modules not found.

- [ ] **Step 3: Implement**

`web/src/pages/application/helpers.ts`:

```ts
import type { Plan } from '../../api/types'

export const appRef = (id: number) => `app_${id}`

export function initials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean)
  return words.slice(0, 2).map(w => w[0]).join('').toUpperCase() || '?'
}

export const frNum = (n: number) => n.toLocaleString('fr-FR')

export function frDate(iso: string): string {
  const d = new Date(iso)
  return isNaN(d.getTime()) ? '—' : d.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' })
}

export function rateLabel(plan: Plan | undefined): string {
  if (!plan) return '—'
  const win = plan.windowSeconds === 60 ? 'min' : `${plan.windowSeconds}s`
  return `${frNum(plan.rateLimit)} / ${win}`
}

export function maskKey(full: string): string {
  if (full.length <= 10) return full
  return full.slice(0, 8) + '•'.repeat(full.length - 10) + full.slice(-2)
}

export function statusPill(status: string): { cls: string; label: string } {
  switch (status) {
    case 'active': return { cls: 'ok', label: 'Active' }
    case 'pending': return { cls: 'warn', label: 'En attente' }
    case 'rejected': return { cls: 'muted', label: 'Rejeté' }
    default: return { cls: 'muted', label: status }
  }
}

// Deterministic per-app glyph gradient (blueprint shows one gradient per app).
const GRADIENTS = [
  'linear-gradient(150deg,var(--c-eng),var(--accent))',
  'linear-gradient(150deg,var(--c-finance),oklch(50% 0.1 170))',
  'linear-gradient(150deg,var(--c-marketing),oklch(58% 0.13 40))',
  'linear-gradient(150deg,var(--c-admin),var(--accent-d))',
]
export const glyphGradient = (id: number) => GRADIENTS[Math.abs(id) % GRADIENTS.length]

export async function copyText(text: string): Promise<void> {
  try { await navigator.clipboard.writeText(text) } catch { /* non-secure context: best effort */ }
}
```

`web/src/pages/application/demo.ts`:

```ts
// ─────────────────────────────────────────────────────────────────────────────
// ALL demo placeholders for data the backend does not provide yet
// (metrics pipeline, sandbox environments, key rotation, activity log).
// When a real backend feature lands, delete its constant here and wire the API.
// Values mirror /application.html so the page matches the blueprint.
// ─────────────────────────────────────────────────────────────────────────────

export type StatDir = 'up' | 'down' | 'flat'

export const DEMO_STATS: ReadonlyArray<{
  icon: 'pulse' | 'calendar' | 'clock' | 'alert'
  label: string
  value: string
  unit?: string
  delta: { dir: StatDir; arrow: 'up' | 'down' | null; text: string }
}> = [
  { icon: 'pulse', label: "Requêtes · aujourd'hui", value: '18 402', delta: { dir: 'up', arrow: 'up', text: '+12,4 % vs hier' } },
  { icon: 'calendar', label: 'Ce mois-ci', value: '421 K', delta: { dir: 'flat', arrow: null, text: 'sur 1 M inclus · 42 %' } },
  { icon: 'clock', label: 'Latence p95', value: '86', unit: 'ms', delta: { dir: 'up', arrow: 'down', text: '-9 ms · plus rapide' } },
  { icon: 'alert', label: "Taux d'erreur", value: '0,21', unit: '%', delta: { dir: 'up', arrow: 'down', text: 'sous le seuil 1 %' } },
]

export const DEMO_FEED: ReadonlyArray<{ icon: 'check' | 'rotate' | 'alert' | 'plus'; lead: string; rest: string; when: string }> = [
  { icon: 'check', lead: 'Abonnement', rest: ' à Inventory API · plan Gold', when: 'il y a 2 h' },
  { icon: 'rotate', lead: 'Clé Sandbox', rest: ' régénérée', when: 'hier · 16:41' },
  { icon: 'alert', lead: 'Pic de débit', rest: ' sur Payments — 280/300 rpm', when: 'hier · 12:08' },
  { icon: 'plus', lead: 'Application créée', rest: '', when: '12 mars 2026' },
]

export const DEMO_CHART = {
  values: [12, 19, 15, 22, 18, 9, 7, 24, 28, 21, 26, 31, 29, 34],
  labels: ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10', '11', '12', '13', '14'],
}

export const DEMO_USAGE_ROWS: ReadonlyArray<{ ini: string; bg: string; name: string; requests: string; share: number; errors: string; errColor: string }> = [
  { ini: 'OR', bg: 'var(--c-marketing)', name: 'Orders API', requests: '248 910', share: 59, errors: '0,14 %', errColor: 'var(--success)' },
  { ini: 'PA', bg: 'var(--c-finance)', name: 'Payments API', requests: '142 305', share: 34, errors: '0,38 %', errColor: 'oklch(58% 0.12 70)' },
  { ini: 'IN', bg: 'var(--c-eng)', name: 'Inventory API', requests: '29 880', share: 7, errors: '0,02 %', errColor: 'var(--success)' },
]

export const DEMO_SANDBOX_KEY = 'ax_test_5e8d2c1f90ab34cd67ef01a2b3c4d5e6'
export const DEMO_ROTATION = { prod: '14 mai 2026', sbx: 'hier' }

// Deterministic demo consumption width (percent) per subscription row.
export const demoBarWidth = (productId: number) => 15 + ((productId * 37) % 80)
export const demoRpm = (productId: number) => 40 + ((productId * 113) % 600)

// Quickstart fallback when the app has no active subscription yet.
export const DEMO_QUICKSTART = { path: '/orders', key: 'ax_live_a3f9c1e7b240d8e5f6...' }

export function demoRotatedKey(prefix: 'ax_test_'): string {
  let s = ''
  for (let i = 0; i < 32; i++) s += '0123456789abcdef'[Math.floor(Math.random() * 16)]
  return prefix + s
}
```

`web/src/pages/application/Toast.tsx`:

```tsx
export function Toast({ msg }: { msg: string | null }) {
  return (
    <div className={`appdetail-toast ${msg ? 'show' : ''}`} role="status" aria-live="polite">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.2} aria-hidden="true">
        <path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span>{msg}</span>
    </div>
  )
}
```

`web/src/pages/application/ConfirmModal.tsx`:

```tsx
import { useEffect } from 'react'

export interface ModalSpec {
  title: string
  body: string
  confirmLabel?: string
  danger?: boolean
  onConfirm: () => void
}

function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function RotateIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function ConfirmModal({ spec, onClose }: { spec: ModalSpec | null; onClose: () => void }) {
  useEffect(() => {
    if (!spec) return
    function onKey(e: KeyboardEvent) { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [spec, onClose])

  if (!spec) return null
  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <div className="dmodal" role="dialog" aria-modal="true" aria-label={spec.title}>
        <div
          className="mi"
          style={{
            background: spec.danger ? 'var(--danger-soft)' : 'var(--warn-soft)',
            color: spec.danger ? 'var(--danger)' : 'oklch(52% 0.14 70)',
          }}
        >
          {spec.danger ? <TrashIcon /> : <RotateIcon />}
        </div>
        <h3>{spec.title}</h3>
        <p>{spec.body}</p>
        <div className="ma">
          <button className="btn ghost" onClick={onClose}>Annuler</button>
          <button
            className={`btn ${spec.danger ? 'danger' : 'primary'}`}
            onClick={() => { const fn = spec.onConfirm; onClose(); fn() }}
          >
            {spec.confirmLabel ?? 'Confirmer'}
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && npx vitest run src/pages/application/`
Expected: PASS (9 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/application/
git commit -m "feat(web): app detail primitives — helpers, demo data, toast, confirm modal"
```

---

### Task 4: `CredentialsTab`

**Files:**
- Create: `web/src/pages/application/CredentialsTab.tsx`
- Test: `web/src/pages/application/CredentialsTab.test.tsx`

- [ ] **Step 1: Write failing tests**

`web/src/pages/application/CredentialsTab.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { CredentialsTab } from './CredentialsTab'
import type { ModalSpec } from './ConfirmModal'

const KEY = 'ax_live_a3f9c1e7b240d8e5f6a1b9c4d7e2f8a0'

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

function setup() {
  const notify = vi.fn()
  let lastModal: ModalSpec | null = null
  const openModal = vi.fn((s: ModalSpec) => { lastModal = s })
  render(<CredentialsTab apiKey={KEY} notify={notify} openModal={openModal} />)
  return { notify, openModal, getModal: () => lastModal }
}

describe('CredentialsTab', () => {
  it('masks the production key by default and reveals on toggle', async () => {
    setup()
    const code = screen.getByTestId('key-prod')
    expect(code.textContent).toBe('ax_live_' + '•'.repeat(KEY.length - 10) + 'a0')
    await userEvent.click(screen.getAllByRole('button', { name: 'Afficher / masquer' })[0])
    expect(code.textContent).toBe(KEY)
  })
  it('copies the real key and notifies', async () => {
    const { notify } = setup()
    await userEvent.click(screen.getAllByRole('button', { name: 'Copier' })[0])
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(KEY)
    expect(notify).toHaveBeenCalledWith('Clé copiée dans le presse-papiers')
  })
  it('production rotate confirms into a coming-soon toast, key untouched', async () => {
    const { notify, openModal, getModal } = setup()
    await userEvent.click(screen.getAllByRole('button', { name: /Régénérer/ })[0])
    expect(openModal).toHaveBeenCalled()
    getModal()!.onConfirm()
    expect(notify).toHaveBeenCalledWith('Rotation des clés à venir')
    expect(screen.getByTestId('key-prod').textContent).toContain('ax_live_')
  })
  it('sandbox rotate visually replaces the demo key', async () => {
    const { getModal } = setup()
    const before = screen.getByTestId('key-sbx').textContent
    await userEvent.click(screen.getAllByRole('button', { name: /Régénérer/ })[1])
    getModal()!.onConfirm()
    const after = screen.getByTestId('key-sbx').textContent
    expect(after).toMatch(/^ax_test_[0-9a-f]{32}$/)   // revealed fresh key
    expect(after).not.toBe(before)
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/application/CredentialsTab.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `web/src/pages/application/CredentialsTab.tsx`**

```tsx
import { useState } from 'react'
import { maskKey, copyText } from './helpers'
import { DEMO_SANDBOX_KEY, DEMO_ROTATION, demoRotatedKey } from './demo'
import type { ModalSpec } from './ConfirmModal'

function EyeIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z" /><circle cx="12" cy="12" r="3" />
    </svg>
  )
}
function CopyIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15V5a2 2 0 012-2h10" strokeLinecap="round" />
    </svg>
  )
}
function RotateIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.9} aria-hidden="true">
      <path d="M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8M3 22v-6h6M21 12a9 9 0 01-15 6.7L3 16" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function KeyCard({ kind, label, tag, fullKey, revealed, onToggle, rotatedAt, onCopy, onRotate, testId }: {
  kind: 'prod' | 'sbx'
  label: string
  tag: string
  fullKey: string
  revealed: boolean
  onToggle: () => void
  rotatedAt: string
  onCopy: () => void
  onRotate: () => void
  testId: string
}) {
  return (
    <div className={`keycard ${kind}`}>
      <div className="kh">
        <span className="env">{label} <span className="envtag">{tag}</span></span>
      </div>
      <div className="kb">
        <div className="keyrow">
          <code data-testid={testId}>{revealed ? fullKey : maskKey(fullKey)}</code>
          <button className="iconbtn" aria-label="Afficher / masquer" onClick={onToggle}><EyeIcon /></button>
          <button className="iconbtn" aria-label="Copier" onClick={onCopy}><CopyIcon /></button>
        </div>
        <div className="keymeta">
          <span>Dernière rotation · <span className="mono">{rotatedAt}</span></span>
          <button className="rotate" onClick={onRotate}><RotateIcon />Régénérer</button>
        </div>
      </div>
    </div>
  )
}

export function CredentialsTab({ apiKey, notify, openModal }: {
  apiKey: string
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
}) {
  const [prodRevealed, setProdRevealed] = useState(false)
  const [sbxRevealed, setSbxRevealed] = useState(false)
  // DEMO: sandbox environments don't exist server-side yet (see demo.ts).
  const [sbxKey, setSbxKey] = useState(DEMO_SANDBOX_KEY)

  function copy(key: string) {
    void copyText(key).then(() => notify('Clé copiée dans le presse-papiers'))
  }

  return (
    <section className="panel">
      <p className="section-title">Clés API · key-auth</p>
      <div className="keygrid">
        <KeyCard
          kind="prod" label="Production" tag="live" testId="key-prod"
          fullKey={apiKey} revealed={prodRevealed} onToggle={() => setProdRevealed(r => !r)}
          rotatedAt={DEMO_ROTATION.prod}
          onCopy={() => copy(apiKey)}
          onRotate={() => openModal({
            title: 'Régénérer la clé production ?',
            body: 'L’ancienne clé serait révoquée immédiatement dans APISIX (consumer key-auth). La rotation de clé arrive bientôt côté portail.',
            confirmLabel: 'Régénérer la clé',
            // Real credential: no backend rotation endpoint yet — never fake it visually.
            onConfirm: () => notify('Rotation des clés à venir'),
          })}
        />
        <KeyCard
          kind="sbx" label="Sandbox" tag="test" testId="key-sbx"
          fullKey={sbxKey} revealed={sbxRevealed} onToggle={() => setSbxRevealed(r => !r)}
          rotatedAt={DEMO_ROTATION.sbx}
          onCopy={() => copy(sbxKey)}
          onRotate={() => openModal({
            title: 'Régénérer la clé sandbox ?',
            body: 'L’ancienne clé sera révoquée immédiatement dans APISIX (consumer key-auth). Les requêtes qui l’utilisent recevront un 401 — pensez à redéployer.',
            confirmLabel: 'Régénérer la clé',
            onConfirm: () => {
              const nk = demoRotatedKey('ax_test_')
              setSbxKey(nk)
              setSbxRevealed(true)   // blueprint reveals the fresh key once
              notify('Nouvelle clé sandbox générée')
            },
          })}
        />
      </div>

      <div className="dcard" style={{ marginTop: 20 }}>
        <div className="ch"><h3>Sécurité de la clé</h3></div>
        <div className="cb" style={{ display: 'flex', gap: 30, flexWrap: 'wrap', fontSize: '13.5px', color: 'var(--muted)', lineHeight: 1.6 }}>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>Ne la partagez jamais côté client</b>La clé porte tous les droits de l'application. Gardez-la côté serveur ou dans un secret manager.</div>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>Régénérer invalide l'ancienne</b>La rotation révoque immédiatement le <span className="mono">consumer</span> précédent dans APISIX. Prévoyez le redéploiement.</div>
          <div style={{ flex: 1, minWidth: 240 }}><b style={{ color: 'var(--fg)', display: 'block', marginBottom: 5 }}>OAuth2 / JWT à venir</b>Le portail est prêt pour un second fournisseur d'identifiants (<span className="mono">jwt-auth</span>) sans réécriture.</div>
        </div>
      </div>
    </section>
  )
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && npx vitest run src/pages/application/CredentialsTab.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/application/CredentialsTab.tsx web/src/pages/application/CredentialsTab.test.tsx
git commit -m "feat(web): credentials tab — real prod key, demo sandbox, rotation flows"
```

---

### Task 5: `SubscriptionsTab`

**Files:**
- Create: `web/src/pages/application/SubscriptionsTab.tsx`
- Test: `web/src/pages/application/SubscriptionsTab.test.tsx`

- [ ] **Step 1: Write failing tests**

`web/src/pages/application/SubscriptionsTab.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { SubscriptionsTab } from './SubscriptionsTab'
import type { SubscriptionView, Plan } from '../../api/types'

const subs: SubscriptionView[] = [
  { productId: 1, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' },
  { productId: 2, productName: 'Inventory API', version: '1.4.0', contextPath: '/inventory', planId: 1, planName: 'Free', status: 'pending' },
]
const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60 },
]

function setup() {
  const onResiliate = vi.fn()
  render(<MemoryRouter><SubscriptionsTab subs={subs} plans={plans} onResiliate={onResiliate} /></MemoryRouter>)
  return { onResiliate }
}

describe('SubscriptionsTab', () => {
  it('renders one row per subscription with real plan rate and status', () => {
    setup()
    expect(screen.getByText('Orders API')).toBeInTheDocument()
    expect(screen.getByText('/orders · v2.1.0')).toBeInTheDocument()
    expect(screen.getByText('1 000 / min')).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('En attente')).toBeInTheDocument()
  })
  it('keeps the blueprint Gérer placeholder', () => {
    setup()
    expect(screen.getAllByText('Gérer')).toHaveLength(2)
  })
  it('résilier delegates to the page callback', async () => {
    const { onResiliate } = setup()
    await userEvent.click(screen.getAllByText('Résilier')[0])
    expect(onResiliate).toHaveBeenCalledWith(1, 'Orders API')
  })
  it('shows the empty state when there are no subscriptions', () => {
    render(<MemoryRouter><SubscriptionsTab subs={[]} plans={plans} onResiliate={() => {}} /></MemoryRouter>)
    expect(screen.getByText(/Aucun abonnement/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/application/SubscriptionsTab.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `web/src/pages/application/SubscriptionsTab.tsx`**

```tsx
import { Link } from 'react-router-dom'
import type { SubscriptionView, Plan } from '../../api/types'
import { initials, rateLabel, statusPill } from './helpers'
import { demoBarWidth, demoRpm } from './demo'

const IG_COLORS = ['var(--c-marketing)', 'var(--c-finance)', 'var(--c-eng)', 'var(--c-admin)']
const PLAN_DOTS: Record<string, string> = { Gold: 'var(--warn)', Silver: 'var(--c-admin)', Free: 'var(--c-finance)' }

function PlusIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
      <path d="M12 5v14M5 12h14" strokeLinecap="round" />
    </svg>
  )
}

export function SubscriptionsTab({ subs, plans, onResiliate }: {
  subs: SubscriptionView[]
  plans: Plan[]
  onResiliate: (productId: number, productName: string) => void
}) {
  return (
    <section className="panel">
      <div className="dcard">
        <div className="ch">
          <div>
            <h3>API abonnées</h3>
            <p>Chaque abonnement lie cette application à une API, à un palier de débit.</p>
          </div>
          <div className="right">
            <Link className="btn primary sm" to="/"><PlusIcon />Abonner une API</Link>
          </div>
        </div>
        <div className="cb" style={{ padding: 0 }}>
          {subs.length === 0 ? (
            <p style={{ padding: 20, fontSize: 14, color: 'var(--muted)' }}>Aucun abonnement. Parcourez le catalogue pour abonner cette application à une API.</p>
          ) : (
            <div className="tblwrap">
              <table className="tbl">
                <thead>
                  <tr><th>API</th><th>Plan</th><th>Débit</th><th>Consommation (rpm)</th><th>Statut</th><th></th></tr>
                </thead>
                <tbody>
                  {subs.map((s, i) => {
                    const pill = statusPill(s.status)
                    const width = demoBarWidth(s.productId)
                    return (
                      <tr key={s.productId}>
                        <td>
                          <div className="apicell">
                            <span className="ig" style={{ background: IG_COLORS[i % IG_COLORS.length] }}>{initials(s.productName)}</span>
                            <span><span className="nm">{s.productName}</span><span className="cx">{s.contextPath} · v{s.version}</span></span>
                          </div>
                        </td>
                        <td><span className="plan-pill"><span className="dot" style={{ background: PLAN_DOTS[s.planName] ?? 'var(--c-finance)' }} />{s.planName}</span></td>
                        <td className="rate">{rateLabel(plans.find(p => p.id === s.planId))}</td>
                        <td>
                          {/* DEMO: no per-subscription metrics yet (see demo.ts) */}
                          <div className={`bar ${width > 85 ? 'hi' : ''}`}><i style={{ width: `${width}%` }} /></div>
                          <div className="rowsub">{demoRpm(s.productId)} rpm · pic 24h</div>
                        </td>
                        <td><span className={`stpill ${pill.cls}`}><span className="led" />{pill.label}</span></td>
                        <td>
                          <div className="rowact">
                            {/* Blueprint placeholder kept per user choice (spec 2026-06-05) */}
                            <a className="linkbtn">Gérer</a>
                            <a className="linkbtn danger" onClick={() => onResiliate(s.productId, s.productName)}>Résilier</a>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && npx vitest run src/pages/application/SubscriptionsTab.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/application/SubscriptionsTab.tsx web/src/pages/application/SubscriptionsTab.test.tsx
git commit -m "feat(web): subscriptions tab — real table with résilier delegation"
```

---

### Task 6: `OverviewTab`, `UsageTab`, `SettingsTab`

**Files:**
- Create: `web/src/pages/application/OverviewTab.tsx`
- Create: `web/src/pages/application/UsageTab.tsx`
- Create: `web/src/pages/application/SettingsTab.tsx`
- Test: `web/src/pages/application/StaticTabs.test.tsx`

- [ ] **Step 1: Write failing tests**

`web/src/pages/application/StaticTabs.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OverviewTab } from './OverviewTab'
import { UsageTab } from './UsageTab'
import { SettingsTab } from './SettingsTab'
import type { AppDetail, Application } from '../../api/types'
import type { ModalSpec } from './ConfirmModal'

beforeEach(() => {
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

const app: Application = { id: 4, ownerId: 1, name: 'Boutique Mobile', description: 'desc app', createdAt: '2026-03-12T00:00:00Z' }
const detail: AppDetail = {
  apiKey: 'ax_live_real_key_0001', consumerUsername: 'app_4',
  subscriptions: [{ productId: 9, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' }],
}

describe('OverviewTab', () => {
  it('renders demo stats and a quickstart with the real gateway path + key', () => {
    const notify = vi.fn()
    render(<OverviewTab detail={detail} notify={notify} />)
    expect(screen.getByText("Requêtes · aujourd'hui")).toBeInTheDocument()
    expect(screen.getByText(/9080\/orders/)).toBeInTheDocument()
    expect(screen.getByText(/ax_live_real_key_0001/)).toBeInTheDocument()
  })
  it('falls back to blueprint sample without an active subscription', () => {
    render(<OverviewTab detail={{ ...detail, subscriptions: [] }} notify={() => {}} />)
    expect(screen.getByText(/ax_live_a3f9c1e7b240d8e5f6/)).toBeInTheDocument()
  })
  it('copy button copies the curl command', async () => {
    const notify = vi.fn()
    render(<OverviewTab detail={detail} notify={notify} />)
    await userEvent.click(screen.getByRole('button', { name: 'Copier' }))
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(expect.stringContaining('ax_live_real_key_0001'))
    expect(notify).toHaveBeenCalledWith('Commande copiée')
  })
})

describe('UsageTab', () => {
  it('renders 14 chart columns and the per-API demo table', () => {
    render(<UsageTab />)
    expect(screen.getAllByTestId('chart-col')).toHaveLength(14)
    expect(screen.getByText('Orders API')).toBeInTheDocument()
    expect(screen.getByText('248 910')).toBeInTheDocument()
  })
})

describe('SettingsTab', () => {
  it('prefills real name/description and saving shows the demo toast', async () => {
    const notify = vi.fn()
    render(<SettingsTab app={app} notify={notify} openModal={() => {}} />)
    expect(screen.getByLabelText("Nom de l'application")).toHaveValue('Boutique Mobile')
    expect(screen.getByLabelText('Description')).toHaveValue('desc app')
    await userEvent.click(screen.getByRole('button', { name: /Enregistrer/ }))
    expect(notify).toHaveBeenCalledWith('Modifications enregistrées (démo)')
  })
  it('delete app goes through the danger modal then demo toast', async () => {
    const notify = vi.fn()
    let lastModal: ModalSpec | null = null
    render(<SettingsTab app={app} notify={notify} openModal={s => { lastModal = s }} />)
    await userEvent.click(screen.getByRole('button', { name: /Supprimer l'application/ }))
    expect(lastModal!.danger).toBe(true)
    lastModal!.onConfirm()
    expect(notify).toHaveBeenCalledWith('Application supprimée (démo)')
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/application/StaticTabs.test.tsx`
Expected: FAIL — modules not found.

- [ ] **Step 3: Implement the three components**

`web/src/pages/application/OverviewTab.tsx`:

```tsx
import type { AppDetail } from '../../api/types'
import { copyText } from './helpers'
import { DEMO_STATS, DEMO_FEED, DEMO_QUICKSTART } from './demo'

const STAT_ICONS: Record<string, string> = {
  pulse: 'M3 12h4l3 8 4-16 3 8h4',
  calendar: 'M3 9h18M8 4v16M3 4h18v16H3z',
  clock: 'M12 7v5l3 2M12 21a9 9 0 100-18 9 9 0 000 18z',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
}
const FEED_ICONS: Record<string, string> = {
  check: 'M20 6L9 17l-5-5',
  rotate: 'M21 2v6h-6M3 12a9 9 0 0115-6.7L21 8M3 22v-6h6M21 12a9 9 0 01-15 6.7L3 16',
  alert: 'M12 9v4M12 17h.01M10.3 4.3L2.5 18a2 2 0 001.7 3h15.6a2 2 0 001.7-3L13.7 4.3a2 2 0 00-3.4 0z',
  plus: 'M12 5v14M5 12h14',
}

function Arrow({ dir }: { dir: 'up' | 'down' }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} aria-hidden="true">
      <path d={dir === 'up' ? 'M6 15l6-6 6 6' : 'M18 9l-6 6-6-6'} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function OverviewTab({ detail, notify }: { detail: AppDetail; notify: (msg: string) => void }) {
  // Quickstart uses the first ACTIVE subscription's real gateway path + real key;
  // the blueprint sample otherwise.
  const active = detail.subscriptions.find(s => s.status === 'active')
  const path = active ? active.contextPath : DEMO_QUICKSTART.path
  const key = active ? detail.apiKey : DEMO_QUICKSTART.key
  const curl = `curl http://localhost:9080${path} -H "apikey: ${key}"`

  function copyCurl() {
    void copyText(curl).then(() => notify('Commande copiée'))
  }

  return (
    <section className="panel">
      {/* DEMO metrics — no metrics pipeline yet (see demo.ts) */}
      <div className="stats">
        {DEMO_STATS.map(s => (
          <div className="stat" key={s.label}>
            <div className="k">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d={STAT_ICONS[s.icon]} strokeLinecap="round" strokeLinejoin="round" /></svg>
              {s.label}
            </div>
            <div className="v">{s.value}{s.unit && <> <small>{s.unit}</small></>}</div>
            <div className={`d ${s.delta.dir}`}>{s.delta.arrow && <Arrow dir={s.delta.arrow} />}{s.delta.text}</div>
          </div>
        ))}
      </div>

      <div className="twocol">
        <div className="dcard">
          <div className="ch">
            <div>
              <h3>Démarrage rapide</h3>
              <p>Authentification par clé API — un seul en-tête <span className="mono">apikey</span>.</p>
            </div>
          </div>
          <div className="cb">
            <div className="code">
              <div className="cbar"><i /><i /><i /><span>requête — production</span>
                <button className="copy" onClick={copyCurl}>Copier</button>
              </div>
              <pre><span className="c"># Un seul en-tête, c'est tout</span>{'\n'}<span className="cmd">curl</span> http://localhost:9080{path} \{'\n'}  <span className="flag">-H</span> <span className="str">"apikey: {key}"</span></pre>
            </div>
            <p style={{ fontSize: 13, color: 'var(--muted)', marginTop: 14, lineHeight: 1.55 }}>
              La clé est liée à un <b style={{ color: 'var(--fg)' }}>consumer</b> APISIX et au plan choisi à l'abonnement (<span className="mono">key-auth</span> + <span className="mono">limit-count</span>). Utilisez la clé <b style={{ color: 'var(--fg)' }}>Sandbox</b> pour tester sans consommer votre quota production.
            </p>
          </div>
        </div>

        <div className="dcard">
          <div className="ch"><h3>Activité récente</h3></div>
          <div className="cb" style={{ paddingTop: 6, paddingBottom: 6 }}>
            {/* DEMO feed — no activity log yet (see demo.ts) */}
            <ul className="feed">
              {DEMO_FEED.map(f => (
                <li key={f.lead + f.when}>
                  <span className="fi"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><path d={FEED_ICONS[f.icon]} strokeLinecap="round" strokeLinejoin="round" /></svg></span>
                  <span className="ft"><b>{f.lead}</b>{f.rest}<small>{f.when}</small></span>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  )
}
```

`web/src/pages/application/UsageTab.tsx`:

```tsx
import { DEMO_CHART, DEMO_USAGE_ROWS } from './demo'
import { frNum } from './helpers'

// Entirely DEMO — no metrics pipeline yet (see demo.ts).
export function UsageTab() {
  const max = Math.max(...DEMO_CHART.values)
  return (
    <section className="panel">
      <div className="dcard">
        <div className="ch">
          <div>
            <h3>Requêtes · 14 derniers jours</h3>
            <p>Toutes API confondues, environnement production.</p>
          </div>
          <div className="right"><span className="stpill muted"><span className="led" />421 K ce mois</span></div>
        </div>
        <div className="cb">
          <div className="chart">
            {DEMO_CHART.values.map((v, i) => (
              <div className="col" key={i} data-testid="chart-col">
                <div className="bw" data-v={`${frNum(v * 1000)} req`} style={{ height: `${Math.round((v / max) * 100)}%` }} />
                <small>{DEMO_CHART.labels[i]}</small>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="dcard" style={{ marginTop: 20 }}>
        <div className="ch"><h3>Répartition par API</h3></div>
        <div className="cb" style={{ padding: 0 }}>
          <table className="tbl">
            <thead><tr><th>API</th><th>Requêtes (mois)</th><th>Part</th><th>Erreurs</th></tr></thead>
            <tbody>
              {DEMO_USAGE_ROWS.map(r => (
                <tr key={r.name}>
                  <td><div className="apicell"><span className="ig" style={{ background: r.bg }}>{r.ini}</span><span className="nm">{r.name}</span></div></td>
                  <td className="mono">{r.requests}</td>
                  <td><div className="bar"><i style={{ width: `${r.share}%` }} /></div></td>
                  <td className="mono" style={{ color: r.errColor }}>{r.errors}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  )
}
```

`web/src/pages/application/SettingsTab.tsx`:

```tsx
import { useState } from 'react'
import type { Application } from '../../api/types'
import type { ModalSpec } from './ConfirmModal'

function CheckIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
      <path d="M20 6L9 17l-5-5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
function TrashIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true">
      <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2M19 6l-1 14a2 2 0 01-2 2H8a2 2 0 01-2-2L5 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function SettingsTab({ app, notify, openModal }: {
  app: Application
  notify: (msg: string) => void
  openModal: (spec: ModalSpec) => void
}) {
  const [name, setName] = useState(app.name)
  const [desc, setDesc] = useState(app.description)

  return (
    <section className="panel">
      <div className="dcard">
        <div className="ch"><h3>Détails de l'application</h3></div>
        <div className="cb">
          <div className="field">
            <label htmlFor="s-name">Nom de l'application</label>
            <input id="s-name" type="text" value={name} onChange={e => setName(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="s-desc">Description</label>
            <textarea id="s-desc" value={desc} onChange={e => setDesc(e.target.value)} />
          </div>
          <div className="field">
            <label htmlFor="s-env">Environnement par défaut</label>
            <select id="s-env" defaultValue="Production">
              <option>Production</option>
              <option>Sandbox</option>
            </select>
            <p className="hint">Détermine la clé pré-sélectionnée dans les exemples de code.</p>
          </div>
          {/* DEMO: no application-update endpoint yet */}
          <button className="btn primary" onClick={() => notify('Modifications enregistrées (démo)')}>
            <CheckIcon />Enregistrer
          </button>
        </div>
      </div>

      <p className="section-title" style={{ marginTop: 26 }}>Zone sensible</p>
      <div className="danger-zone">
        <div className="dz-t">
          <h4>Supprimer cette application</h4>
          <p>Révoque toutes les clés et résilie les abonnements. Irréversible.</p>
        </div>
        {/* Blueprint demo behavior — no delete endpoint yet */}
        <button
          className="btn danger"
          onClick={() => openModal({
            title: `Supprimer « ${app.name} » ?`, danger: true, confirmLabel: 'Supprimer définitivement',
            body: 'Toutes les clés seront révoquées et les abonnements résiliés dans APISIX. Cette action est irréversible.',
            onConfirm: () => notify('Application supprimée (démo)'),
          })}
        >
          <TrashIcon />Supprimer l'application
        </button>
      </div>
    </section>
  )
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && npx vitest run src/pages/application/StaticTabs.test.tsx`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/application/OverviewTab.tsx web/src/pages/application/UsageTab.tsx web/src/pages/application/SettingsTab.tsx web/src/pages/application/StaticTabs.test.tsx
git commit -m "feat(web): overview/usage/settings tabs (demo metrics, real quickstart)"
```

---

### Task 7: `AppSwitcher` + `CreateAppModal`

**Files:**
- Create: `web/src/pages/application/AppSwitcher.tsx`
- Test: `web/src/pages/application/AppSwitcher.test.tsx`

- [ ] **Step 1: Write failing tests**

`web/src/pages/application/AppSwitcher.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { AppSwitcher, CreateAppModal } from './AppSwitcher'
import type { Application } from '../../api/types'

const apps: Application[] = [
  { id: 1, ownerId: 1, name: 'Boutique Mobile', description: '', createdAt: '2026-03-12T00:00:00Z' },
  { id: 2, ownerId: 1, name: 'Analytics interne', description: '', createdAt: '2026-04-02T00:00:00Z' },
]

describe('AppSwitcher', () => {
  it('opens the menu listing all apps, current one marked', async () => {
    render(<MemoryRouter><AppSwitcher apps={apps} currentId={1} onCreate={() => {}} /></MemoryRouter>)
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    expect(screen.getByText('Analytics interne')).toBeInTheDocument()
    expect(screen.getByText('Boutique Mobile').closest('a')).toHaveClass('cur')
    expect(screen.getByText('app_2')).toBeInTheDocument()
  })
  it('exposes the Nouvelle application action', async () => {
    const onCreate = vi.fn()
    render(<MemoryRouter><AppSwitcher apps={apps} currentId={1} onCreate={onCreate} /></MemoryRouter>)
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    await userEvent.click(screen.getByText('Nouvelle application'))
    expect(onCreate).toHaveBeenCalled()
  })
})

describe('CreateAppModal', () => {
  it('creates with the typed name', async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined)
    render(<CreateAppModal open onClose={() => {}} onCreate={onCreate} />)
    await userEvent.type(screen.getByLabelText("Nom de l'application"), 'Mon App')
    await userEvent.click(screen.getByRole('button', { name: 'Créer' }))
    expect(onCreate).toHaveBeenCalledWith('Mon App')
  })
  it('renders nothing when closed', () => {
    const { container } = render(<CreateAppModal open={false} onClose={() => {}} onCreate={async () => {}} />)
    expect(container.firstChild).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/application/AppSwitcher.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `web/src/pages/application/AppSwitcher.tsx`**

```tsx
import { useEffect, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import type { Application } from '../../api/types'
import { appRef, initials, glyphGradient } from './helpers'

export function AppSwitcher({ apps, currentId, onCreate }: {
  apps: Application[]
  currentId: number
  onCreate: () => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLSpanElement>(null)

  useEffect(() => {
    if (!open) return
    function onDoc(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [open])

  return (
    <span className={`switch ${open ? 'open' : ''}`} ref={ref}>
      <button className="trigger" onClick={() => setOpen(o => !o)} aria-haspopup="menu" aria-expanded={open}>
        Changer d'application
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M6 9l6 6 6-6" strokeLinecap="round" strokeLinejoin="round" /></svg>
      </button>
      <div className="menu" role="menu">
        {apps.map(a => (
          <Link key={a.id} to={`/applications/${a.id}`} className={a.id === currentId ? 'cur' : ''} onClick={() => setOpen(false)} role="menuitem">
            <span className="mg" style={{ background: glyphGradient(a.id) }}>{initials(a.name)}</span>
            <span className="mt">{a.name}<small>{appRef(a.id)}</small></span>
          </Link>
        ))}
        <div className="div" />
        <a className="new" onClick={() => { setOpen(false); onCreate() }} role="menuitem">
          <span className="mg" style={{ background: 'var(--accent-soft)', color: 'var(--accent)' }}>+</span>
          <span className="mt">Nouvelle application</span>
        </a>
      </div>
    </span>
  )
}

export function CreateAppModal({ open, onClose, onCreate }: {
  open: boolean
  onClose: () => void
  onCreate: (name: string) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)

  if (!open) return null

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setBusy(true)
    try { await onCreate(name.trim()); setName('') } finally { setBusy(false) }
  }

  return (
    <div className="appdetail-scrim" onClick={e => { if (e.target === e.currentTarget) onClose() }}>
      <form className="dmodal" onSubmit={submit}>
        <h3>Nouvelle application</h3>
        <p>Une application porte sa propre clé d'API et ses abonnements.</p>
        <div className="field">
          <label htmlFor="new-app-name">Nom de l'application</label>
          <input id="new-app-name" value={name} onChange={e => setName(e.target.value)} placeholder="Ex. Boutique Mobile" autoFocus />
        </div>
        <div className="ma">
          <button type="button" className="btn ghost" onClick={onClose}>Annuler</button>
          <button type="submit" className="btn primary" disabled={busy || !name.trim()}>Créer</button>
        </div>
      </form>
    </div>
  )
}
```

- [ ] **Step 4: Run tests**

Run: `cd web && npx vitest run src/pages/application/AppSwitcher.test.tsx`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/pages/application/AppSwitcher.tsx web/src/pages/application/AppSwitcher.test.tsx
git commit -m "feat(web): app switcher dropdown + create-application modal"
```

---

### Task 8: `AppDetailPage` shell + routing (replaces old page)

**Files:**
- Create: `web/src/pages/application/AppDetailPage.tsx`
- Create: `web/src/pages/application/ApplicationsIndex.tsx`
- Modify: `web/src/App.tsx`
- Delete: `web/src/pages/ApplicationsPage.tsx`, `web/src/pages/ApplicationsPage.test.tsx`
- Test: `web/src/pages/application/AppDetailPage.test.tsx`

- [ ] **Step 1: Write failing tests**

`web/src/pages/application/AppDetailPage.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AppDetailPage } from './AppDetailPage'
import { ApplicationsIndex } from './ApplicationsIndex'
import { AuthProvider } from '../../auth/AuthProvider'
import * as api from '../../api/client'
import type { Application, AppDetail, Plan } from '../../api/types'

const apps: Application[] = [
  { id: 1, ownerId: 1, name: 'Boutique Mobile', description: 'desc', createdAt: '2026-03-12T00:00:00Z' },
  { id: 2, ownerId: 1, name: 'Analytics interne', description: '', createdAt: '2026-04-02T00:00:00Z' },
]
const detail: AppDetail = {
  apiKey: 'ax_live_k1', consumerUsername: 'app_1',
  subscriptions: [
    { productId: 9, productName: 'Orders API', version: '2.1.0', contextPath: '/orders', planId: 3, planName: 'Gold', status: 'active' },
    { productId: 5, productName: 'Inventory API', version: '1.4.0', contextPath: '/inventory', planId: 1, planName: 'Free', status: 'pending' },
  ],
}
const plans: Plan[] = [
  { id: 1, name: 'Free', rateLimit: 60, windowSeconds: 60 },
  { id: 3, name: 'Gold', rateLimit: 1000, windowSeconds: 60 },
]

beforeEach(() => {
  localStorage.clear()
  localStorage.setItem('token', 'jwt')
  localStorage.setItem('user', JSON.stringify({ id: 1, email: 'a@b.c', name: 'Dev', role: 'developer' }))
  vi.restoreAllMocks()
  vi.spyOn(api, 'getApplications').mockResolvedValue(apps)
  vi.spyOn(api, 'getApplicationDetail').mockResolvedValue(detail)
  vi.spyOn(api, 'getPlans').mockResolvedValue(plans)
  Object.assign(navigator, { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
})

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <Routes>
          <Route path="/applications" element={<ApplicationsIndex />} />
          <Route path="/applications/:id" element={<AppDetailPage />} />
          <Route path="/" element={<div>CATALOG</div>} />
          <Route path="/login" element={<div>LOGIN</div>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>
  )
}

describe('ApplicationsIndex', () => {
  it('redirects to the first application', async () => {
    renderAt('/applications')
    await waitFor(() => expect(screen.getByText('Boutique Mobile', { selector: 'h1, h1 *' })).toBeInTheDocument())
  })
  it('shows the create form when no apps exist', async () => {
    vi.spyOn(api, 'getApplications').mockResolvedValue([])
    renderAt('/applications')
    expect(await screen.findByText(/Créez votre première application/)).toBeInTheDocument()
  })
})

describe('AppDetailPage', () => {
  it('renders header with real name, id ref, status pill and subs badge', async () => {
    renderAt('/applications/1')
    expect(await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })).toBeInTheDocument()
    expect(screen.getByText('app_1')).toBeInTheDocument()
    expect(screen.getByText('2 abonnements')).toBeInTheDocument()
    expect(await screen.findByText('Créée le', { exact: false })).toBeInTheDocument()
  })
  it('switches tabs and persists to localStorage', async () => {
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(screen.getByRole('button', { name: /^Identifiants$/ }))
    expect(screen.getByTestId('key-prod')).toBeInTheDocument()
    expect(localStorage.getItem('app:tab')).toBe('creds')
  })
  it('résilier flow: modal → confirm → api → refetch', async () => {
    const unsub = vi.spyOn(api, 'unsubscribe').mockResolvedValue(undefined)
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(screen.getByRole('button', { name: /Abonnements/ }))
    await userEvent.click(screen.getAllByText('Résilier')[0])
    expect(screen.getByText(/Résilier l'abonnement à Orders API/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Résilier' }))
    await waitFor(() => expect(unsub).toHaveBeenCalledWith('jwt', 1, 9))
    expect(api.getApplicationDetail).toHaveBeenCalledTimes(2)
  })
  it('create app from switcher navigates to the new app', async () => {
    const created: Application = { id: 7, ownerId: 1, name: 'Nouvelle', description: '', createdAt: '2026-06-05T00:00:00Z' }
    vi.spyOn(api, 'createApplication').mockResolvedValue(created)
    renderAt('/applications/1')
    await screen.findByRole('heading', { level: 1, name: /Boutique Mobile/ })
    await userEvent.click(screen.getByRole('button', { name: /Changer d'application/ }))
    await userEvent.click(screen.getByText('Nouvelle application'))
    await userEvent.type(screen.getByLabelText("Nom de l'application"), 'Nouvelle')
    await userEvent.click(screen.getByRole('button', { name: 'Créer' }))
    await waitFor(() => expect(api.createApplication).toHaveBeenCalledWith('jwt', 'Nouvelle', ''))
  })
  it('unknown app id redirects to /applications', async () => {
    renderAt('/applications/999')
    await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: /Boutique Mobile/ })).toBeInTheDocument())
  })
})
```

NOTE: check how `AuthProvider` persists its session before writing the localStorage seeding — read `web/src/auth/AuthProvider.tsx` and use its exact storage keys (the test above assumes `token` and `user`; adjust to the real keys if they differ, and mention the adjustment in your report).

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/pages/application/AppDetailPage.test.tsx`
Expected: FAIL — modules not found.

- [ ] **Step 3: Implement `web/src/pages/application/ApplicationsIndex.tsx`**

```tsx
import { useEffect, useState, type FormEvent } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { getApplications, createApplication } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { TopBar } from '../../components/TopBar'
import type { Application } from '../../api/types'
import '../../styles/appdetail.css'

export function ApplicationsIndex() {
  const { token } = useAuth()
  const nav = useNavigate()
  const [apps, setApps] = useState<Application[] | null>(null)
  const [name, setName] = useState('')
  const [err, setErr] = useState('')

  useEffect(() => {
    if (!token) return
    getApplications(token).then(setApps).catch(() => setErr('Impossible de charger les applications.'))
  }, [token])

  if (!token) return <Navigate to="/login" replace />

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!token || !name.trim()) return
    try {
      const a = await createApplication(token, name.trim(), '')
      nav(`/applications/${a.id}`)
    } catch {
      setErr("Création impossible. Réessayez.")
    }
  }

  if (apps && apps.length > 0) return <Navigate to={`/applications/${apps[0].id}`} replace />

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="appdetail">
        {err && <p className="autherr" role="alert">{err}</p>}
        {apps && apps.length === 0 && (
          <div className="dcard" style={{ maxWidth: 520, margin: '40px auto', padding: 26 }}>
            <h3 style={{ fontFamily: 'var(--font-display)', fontSize: 19, fontWeight: 700 }}>Créez votre première application</h3>
            <p style={{ fontSize: 14, color: 'var(--muted)', marginTop: 8, lineHeight: 1.5 }}>
              Une application porte sa clé d'API et ses abonnements aux API du catalogue.
            </p>
            <form onSubmit={onCreate} style={{ display: 'flex', gap: 10, marginTop: 18 }}>
              <input
                aria-label="Nom de la nouvelle application" placeholder="Nom de la nouvelle application"
                value={name} onChange={e => setName(e.target.value)}
                style={{ flex: 1, height: 40, padding: '0 12px', border: '1px solid var(--border-2)', borderRadius: 10, background: 'var(--bg)', color: 'var(--fg)' }}
              />
              <button className="btn primary" type="submit">Créer</button>
            </form>
          </div>
        )}
      </div>
    </>
  )
}
```

- [ ] **Step 4: Implement `web/src/pages/application/AppDetailPage.tsx`**

```tsx
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { getApplications, getApplicationDetail, getPlans, createApplication, unsubscribe } from '../../api/client'
import { useAuth } from '../../auth/AuthProvider'
import { TopBar } from '../../components/TopBar'
import type { Application, AppDetail, Plan } from '../../api/types'
import { appRef, initials, frDate, glyphGradient, statusPill } from './helpers'
import { AppSwitcher, CreateAppModal } from './AppSwitcher'
import { ConfirmModal, type ModalSpec } from './ConfirmModal'
import { Toast } from './Toast'
import { OverviewTab } from './OverviewTab'
import { CredentialsTab } from './CredentialsTab'
import { SubscriptionsTab } from './SubscriptionsTab'
import { UsageTab } from './UsageTab'
import { SettingsTab } from './SettingsTab'
import '../../styles/appdetail.css'

type TabKey = 'overview' | 'creds' | 'subs' | 'usage' | 'settings'
const TAB_KEYS: TabKey[] = ['overview', 'creds', 'subs', 'usage', 'settings']
const TAB_LABELS: Record<TabKey, string> = {
  overview: 'Aperçu', creds: 'Identifiants', subs: 'Abonnements', usage: 'Utilisation', settings: 'Paramètres',
}

function initialTab(): TabKey {
  try {
    const saved = localStorage.getItem('app:tab') as TabKey | null
    return saved && TAB_KEYS.includes(saved) ? saved : 'overview'
  } catch { return 'overview' }
}

export function AppDetailPage() {
  const { token } = useAuth()
  const { id } = useParams()
  const nav = useNavigate()
  const appId = Number(id)

  const [apps, setApps] = useState<Application[] | null>(null)
  const [detail, setDetail] = useState<AppDetail | null>(null)
  const [plans, setPlans] = useState<Plan[]>([])
  const [tab, setTabState] = useState<TabKey>(initialTab)
  const [toastMsg, setToastMsg] = useState<string | null>(null)
  const [modal, setModal] = useState<ModalSpec | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [err, setErr] = useState('')
  const toastTimer = useRef<ReturnType<typeof setTimeout>>(undefined)

  const notify = useCallback((msg: string) => {
    setToastMsg(msg)
    clearTimeout(toastTimer.current)
    toastTimer.current = setTimeout(() => setToastMsg(null), 1900)
  }, [])

  function setTab(t: TabKey) {
    setTabState(t)
    try { localStorage.setItem('app:tab', t) } catch { /* private mode */ }
  }

  const reloadDetail = useCallback(() => {
    if (!token || !Number.isFinite(appId)) return
    getApplicationDetail(token, appId).then(setDetail).catch(() => setErr("Impossible de charger l'application."))
  }, [token, appId])

  useEffect(() => {
    if (!token) return
    getApplications(token).then(setApps).catch(() => setErr('Impossible de charger les applications.'))
    getPlans().then(setPlans).catch(() => { /* rates show as — */ })
  }, [token])

  useEffect(() => { setDetail(null); setErr(''); reloadDetail() }, [reloadDetail])

  if (!token) return <Navigate to="/login" replace />
  if (apps && !apps.some(a => a.id === appId)) return <Navigate to="/applications" replace />

  const app = apps?.find(a => a.id === appId) ?? null
  const subs = detail?.subscriptions ?? []
  const overall = subs.some(s => s.status === 'active')
    ? { cls: 'ok', label: 'Active' }
    : subs.some(s => s.status === 'pending')
      ? { cls: 'warn', label: 'En attente' }
      : { cls: 'muted', label: 'Sans abonnement' }

  function onResiliate(productId: number, productName: string) {
    setModal({
      title: `Résilier l'abonnement à ${productName} ?`,
      body: `L'application perdra l'accès à ${productName}. La clé reste valide pour vos autres abonnements.`,
      confirmLabel: 'Résilier', danger: true,
      onConfirm: () => {
        if (!token) return
        unsubscribe(token, appId, productId)
          .then(() => { notify(`Abonnement à ${productName} résilié`); reloadDetail() })
          .catch(() => notify('Échec de la résiliation'))
      },
    })
  }

  async function onCreateApp(name: string) {
    if (!token) return
    const a = await createApplication(token, name, '')
    setCreateOpen(false)
    notify('Application créée')
    const next = await getApplications(token)
    setApps(next)
    nav(`/applications/${a.id}`)
  }

  return (
    <>
      <TopBar search="" onSearch={() => {}} />
      <div className="appdetail">
        <div className="crumbs">
          <Link to="/">Portail</Link>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M9 6l6 6-6 6" strokeLinecap="round" strokeLinejoin="round" /></svg>
          <Link to="/applications">Applications</Link>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M9 6l6 6-6 6" strokeLinecap="round" strokeLinejoin="round" /></svg>
          <span style={{ color: 'var(--fg)', fontWeight: 500 }}>{app?.name ?? '…'}</span>
        </div>

        {err && <p className="autherr" role="alert">{err}</p>}

        {app && (
          <div className="apphead">
            <div className="glyph" style={{ background: glyphGradient(app.id) }}>{initials(app.name)}</div>
            <div className="htext">
              <h1>
                {app.name}
                <span className={`stpill ${overall.cls}`}><span className="led" />{overall.label}</span>
              </h1>
              <div className="meta">
                <span>ID&nbsp;<span className="mono">{appRef(app.id)}</span></span>
                <span className="sep" />
                <span>{subs.length} abonnement{subs.length > 1 ? 's' : ''}</span>
                <span className="sep" />
                <span>Créée le <span className="mono">{frDate(app.createdAt)}</span></span>
                <span className="sep" />
                {apps && <AppSwitcher apps={apps} currentId={app.id} onCreate={() => setCreateOpen(true)} />}
              </div>
            </div>
            <div className="actions">
              <button className="btn ghost" onClick={() => setTab('settings')}>
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} aria-hidden="true"><circle cx="12" cy="12" r="3" /><path d="M12 3v2.5M12 18.5V21M21 12h-2.5M5.5 12H3M18 6l-1.8 1.8M7.8 16.2L6 18M18 18l-1.8-1.8M7.8 7.8L6 6" strokeLinecap="round" /></svg>
                Paramètres
              </button>
              {tab !== 'subs' && (
                <button className="btn primary" onClick={() => nav('/')}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true"><path d="M12 5v14M5 12h14" strokeLinecap="round" /></svg>
                  Abonner une API
                </button>
              )}
            </div>
          </div>
        )}

        <div className="tabs">
          {TAB_KEYS.map(k => (
            <button key={k} className={tab === k ? 'on' : ''} onClick={() => setTab(k)}>
              {TAB_LABELS[k]}
              {k === 'subs' && <span className="badge">{subs.length}</span>}
            </button>
          ))}
        </div>

        {detail && app && (
          <>
            {tab === 'overview' && <OverviewTab detail={detail} notify={notify} />}
            {tab === 'creds' && <CredentialsTab apiKey={detail.apiKey} notify={notify} openModal={setModal} />}
            {tab === 'subs' && <SubscriptionsTab subs={subs} plans={plans} onResiliate={onResiliate} />}
            {tab === 'usage' && <UsageTab />}
            {tab === 'settings' && <SettingsTab app={app} notify={notify} openModal={setModal} />}
          </>
        )}
      </div>

      <Toast msg={toastMsg} />
      <ConfirmModal spec={modal} onClose={() => setModal(null)} />
      <CreateAppModal open={createOpen} onClose={() => setCreateOpen(false)} onCreate={onCreateApp} />
    </>
  )
}
```

- [ ] **Step 5: Update routes in `web/src/App.tsx`**

Replace the ApplicationsPage import and route:

```tsx
import { Routes, Route } from 'react-router-dom'
import { CatalogPage } from './pages/CatalogPage'
import { LoginPage } from './pages/LoginPage'
import { RegisterPage } from './pages/RegisterPage'
import { ApplicationsIndex } from './pages/application/ApplicationsIndex'
import { AppDetailPage } from './pages/application/AppDetailPage'
import { AdminGuard } from './admin/AdminGuard'
import { AdminProductsPage } from './pages/AdminProductsPage'
import { AdminPlansPage } from './pages/AdminPlansPage'
import { AdminApprovalsPage } from './pages/AdminApprovalsPage'

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<CatalogPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route path="/register" element={<RegisterPage />} />
      <Route path="/applications" element={<ApplicationsIndex />} />
      <Route path="/applications/:id" element={<AppDetailPage />} />
      <Route path="/admin/products" element={<AdminGuard><AdminProductsPage /></AdminGuard>} />
      <Route path="/admin/plans" element={<AdminGuard><AdminPlansPage /></AdminGuard>} />
      <Route path="/admin/approvals" element={<AdminGuard><AdminApprovalsPage /></AdminGuard>} />
    </Routes>
  )
}
```

- [ ] **Step 6: Delete the old page**

```bash
git rm web/src/pages/ApplicationsPage.tsx web/src/pages/ApplicationsPage.test.tsx
```

- [ ] **Step 7: Run the new tests, then the full suite**

Run: `cd web && npx vitest run src/pages/application/AppDetailPage.test.tsx`
Expected: PASS (7 tests).
Run: `cd web && npx vitest run`
Expected: ALL pass; zero failures (the old ApplicationsPage suite is gone, ~20 new tests exist across the page dir).

- [ ] **Step 8: Commit**

```bash
git add web/src/App.tsx web/src/pages/application/
git commit -m "feat(web): application detail page per application.html blueprint, replaces ApplicationsPage"
```

---

### Task 9: Full verification (suite + browser)

**Files:** none (verification only)

- [ ] **Step 1: Full test suite + typecheck + build**

Run: `cd web && npx vitest run && npx tsc --noEmit && npm run build`
Expected: all tests pass, tsc clean, build succeeds.

- [ ] **Step 2: Visual verification against the blueprint**

With the stack running (portal `:8090`, vite `:5174 --strictPort`, `PORTAL_PROXY=http://localhost:8090`):
1. Log in, open `/applications` — must land on the first app's detail.
2. Compare each tab against `http://localhost:8888/application.html` (serve the blueprint with `python3 -m http.server 8888` at repo root): header, pills, tabs, stat cards, code block, key cards, table, chart, settings, danger zone.
3. Exercise: key reveal/copy (toast), sandbox rotate (modal + fresh key), prod rotate ("Rotation des clés à venir"), résilier (modal → row disappears → badge updates), switcher navigation, create app, tab persistence on reload.
4. Toggle dark mode (`document.documentElement.dataset.theme='dark'`) — table hover, pills, bars, danger zone must stay readable.
5. Resize to 880px and 560px — grids collapse per blueprint.

Expected: no invisible elements (the `.card`/`.dcard` trap), no style leakage into catalog/admin pages (spot-check `/` and `/admin/products`).

- [ ] **Step 3: Commit any verification fixes**

If fixes were needed, commit them with messages describing the actual defect found.

---

## Self-review notes (already applied)

- **Spec coverage:** routing+switcher (T7/T8), real-vs-demo map (T4–T6 mark every DEMO with a comment + demo.ts), prod-rotation safety (T4), Gérer placeholder kept (T5), createdAt/description real (T6/T8), `.authcard`-style cleanup N/A here but old page deleted (T8), tokens (T1), scoped CSS + collision grep (T2), dark mode + responsive (T2, T9).
- **Type consistency:** `ModalSpec` defined once (T3) and imported everywhere; tab props match the shell's usage; `demoRotatedKey('ax_test_')` matches its literal-typed parameter.
- **Known judgment calls:** toast/scrim classes are page-unique top-level (`.appdetail-toast`/`.appdetail-scrim`) since fixed overlays; React conditional rendering replaces blueprint `.panel.on` display toggling; localStorage key `app:tab` kept from blueprint.
