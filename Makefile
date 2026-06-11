.PHONY: up down run test test-e2e e2e test-it tidy
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
tidy:        ; go mod tidy
