# Traffic Metrics Pipeline — Plan (follow-up to the activity feed)

**Date:** 2026-06-11
**Status:** Steps 1–2 implemented. Step 3 (frontend) and step 4 (E2E) remain.

## Implementation notes (added 2026-06-14)

- **Step 1 (stack):** done — APISIX `prometheus` plugin enabled globally
  (`EnsureGlobalPrometheus`), metrics bound to `0.0.0.0:9091` in the container,
  and an internal-only `prometheus` service scrapes it. Verified: the `consumer`
  label is emitted and equals the portal's `app_<id>` consumer username, so
  per-app attribution works; latency buckets are in milliseconds.
- **Step 2 (backend):** done, but built on the **cache-in-front-of-bounded-
  queries** option (the endpoint section below), *not* the rollup-job +
  `app_metrics_summary` table the rollout list sketched. The architectural rule
  still holds — metrics never touch the portal's Postgres path: `internal/metrics`
  reads only from Prometheus and an in-memory short-TTL (15s) cache keyed by
  `(consumer, range)`. Every query is bounded (range is an enum; chart step caps
  points), so no migration and no background goroutine were needed. If sustained
  high traffic later makes even the cached miss too costly, promote to recording
  rules without changing the endpoint contract. The `consumer` label is
  validated against `^[A-Za-z0-9_]+$` before interpolation (PromQL-injection
  defense-in-depth). Counts use duration-back-from-now windows (Prometheus has
  no calendar alignment), so in a bursty dev stack `increase()` reads 0 when the
  counter is flat — correct, not a bug; continuous production traffic reads true.
**Context:** The application Overview page has four stat cards (Requêtes
aujourd'hui, Ce mois-ci, Latence p95, Taux d'erreur) and a Usage tab — all
still `DEMO_STATS` / `DEMO_USAGE_ROWS` / `DEMO_CHART` placeholders. The activity
feed beside them is now real (see `internal/events`, `activity.ts`,
commit `feat(activity): real application activity feed`). This plan covers
making the **traffic metrics** real without coupling the portal's request path
to a heavy analytics query.

## The one rule that shapes everything

**Metrics never touch the portal's synchronous DB path.** The portal's CRUD is
low-cardinality and cheap; request-traffic metrics are high-volume time-series.
If a page load triggers an aggregate over a per-request events table — or a
synchronous Prometheus query — page latency becomes coupled to traffic volume.
The read model (what the page reads) must be separate from where requests are
counted. Everything below follows from that.

## Where the data comes from — don't count in the portal

APISIX already emits per-route/per-consumer metrics natively. Enable its
`prometheus` plugin and let Prometheus (or VictoriaMetrics) be the time-series
store. The portal is a **read-only consumer** of pre-aggregated series; it never
tallies requests itself.

- Enable the `prometheus` plugin globally in `deploy/apisix/config.yaml` and add
  a `prometheus` service to `docker-compose.yml` scraping APISIX's
  `:9091/apisix/prometheus/metrics`.
- The metrics we can source directly: `apisix_http_status` (counter, labeled by
  route/consumer/code → request counts + error rate), and the APISIX latency
  histogram (`apisix_http_latency_bucket` → p95 via `histogram_quantile`).
- **Label cardinality is the cost center.** Per-consumer labels are what let us
  attribute traffic to an application. Confirm APISIX is configured to emit the
  consumer label, and cap label sets (no per-request-id labels, ever).
- **Security:** Prometheus and the APISIX metrics port stay internal — never
  published to the host like the admin port was before C1. The portal reaches
  Prometheus over the compose network only.

## The read model — pre-aggregate, never compute on read

The stat cards want rollups, not raw series scanned per page load.

- **Card numbers** (today, this month, p95, error rate): served by Prometheus
  **recording rules** that maintain per-app rollups, or a small background job
  in the portal that scrapes the Prometheus query API on a fixed cadence
  (e.g. every 30s) and writes a one-row-per-app summary to a Postgres
  `app_metrics_summary` table. The page reads that one tiny row. Page-load cost
  is then **constant regardless of traffic volume** — the whole point.
- **Usage chart** (14-point series): downsample to per-hour or per-day buckets
  server-side. Return ~100 points max, never raw per-request data.
- The portal's metrics code path is: cache/rollup read → JSON. It must never
  issue an unbounded Prometheus range query in the request handler.

## The endpoint

`GET /api/applications/{id}/usage?range=24h|7d|30d`

- Reuses the **same owner-scoping** as the detail handler (the `owns` check) and
  the `no-store` header — a usage response is per-tenant data.
- `range` is an **allow-listed enum**, not a free-form duration — an unbounded
  range param is a DoS lever (and a Prometheus-query injection surface if
  interpolated). Reject anything off the list with 400.
- Response: `{ summary: {requestsToday, monthToDate, p95Ms, errorRate}, series:
  [{t, requests, errors}] }`.
- Backed by the cached summary + downsampled series; a short TTL (10–30s) cache
  in front so repeated loads don't fan out to Prometheus.

## Frontend

- Load the metrics panel **asynchronously after** the page shell renders — the
  Overview must not block on the usage call. Show skeleton/loading state in the
  four cards and the chart; the activity feed (already real, already in the
  detail payload) renders immediately.
- New `getUsage(token, appId, range)` in `client.ts` (goes through the same
  `parse` path, so it inherits the 401→logout handling).
- Replace `DEMO_STATS` in `OverviewTab` and `DEMO_CHART`/`DEMO_USAGE_ROWS` in
  `UsageTab` with the fetched data; delete those constants from `demo.ts` (the
  file's header comment already says to delete each demo constant when its real
  feature lands). After this, `demo.ts` is empty except the quickstart fallback.

## Bound everything (no silent caps)

- `range` enum-capped (above); series point count capped by bucket size.
- Cache TTL bounds Prometheus load.
- If the metrics backend is unreachable, the cards show a explicit "métriques
  indisponibles" state — **not** silently fall back to demo numbers (that would
  read as real data when it isn't).

## Rollout order

1. Stack: enable APISIX prometheus plugin + add Prometheus to compose (internal
   only). Verify metrics scrape.
2. Backend: rollup job + `app_metrics_summary` table (migration) + cached
   `/usage` endpoint with the enum-bounded range and owner scoping. Unit-test
   the range validation and the summary mapping; gate the Prometheus-client
   integration behind a flag like the existing `RUN_APISIX_IT`.
3. Frontend: async panel + `getUsage` + replace the demo constants; vitest for
   the loading/empty/error states.
4. E2E: drive traffic through the gateway in the harness, assert the summary
   reflects it (or stub the metrics source for determinism).

## Explicitly out of scope here

- Real-time streaming/websocket metrics (polling the cached summary is enough).
- Per-endpoint (sub-route) breakdowns beyond the per-product Usage table.
- Sandbox-vs-production split (the Sandbox key itself is still demo).
- Long-term metrics retention/rollup-of-rollups — Prometheus retention config is
  a deployment concern.
