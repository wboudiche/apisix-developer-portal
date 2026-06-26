.PHONY: up down run test test-e2e e2e test-it test-e2e-web tidy
up:          ; docker compose up -d
down:        ; docker compose down
# PORTAL_ENV=dev: unset is treated as production, which refuses the built-in
# dev secrets. UPSTREAM_ALLOW_PRIVATE=1: lets products target docker-internal
# upstreams (echo:8080), blocked by the SSRF guard otherwise.
run:         ; PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 go run ./cmd/portal
test:        ; go test ./internal/... ./cmd/...
test-e2e:    ; PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/... -count=1 -v
e2e:         ; docker compose up -d && sleep 12 && PORTAL_ENV=dev UPSTREAM_ALLOW_PRIVATE=1 RUN_E2E=1 go test ./internal/e2e/... -count=1 -v
test-it:     ; RUN_APISIX_IT=1 go test ./internal/apisix/... -run Integration -count=1 -v
# Full-stack Playwright pagination suite: an isolated Postgres (compose project
# portal-e2e on :5433) plus the Go API (:8090) and Vite (:5173), the last two
# started by Playwright's webServer. The DB is brought up healthy first (the API
# has no connect retry), the suite runs, then the DB is torn down — preserving
# the suite's exit code.
test-e2e-web:
	docker compose -p portal-e2e -f docker-compose.e2e.yml up -d --wait
	@-fuser -k $${E2E_API_PORT:-8090}/tcp 2>/dev/null; true
	@-fuser -k $${E2E_WEB_PORT:-5173}/tcp 2>/dev/null; true
	( cd web && CI=1 npm run test:e2e ); status=$$?; \
	  docker compose -p portal-e2e -f docker-compose.e2e.yml down -v; \
	  exit $$status
tidy:        ; go mod tidy
