.PHONY: up down run test test-e2e e2e test-it tidy
up:          ; docker compose up -d
down:        ; docker compose down
run:         ; go run ./cmd/portal
test:        ; go test ./internal/... ./cmd/...
test-e2e:    ; RUN_E2E=1 go test ./internal/e2e/... -count=1 -v
e2e:         ; docker compose up -d && sleep 12 && RUN_E2E=1 go test ./internal/e2e/... -count=1 -v
test-it:     ; RUN_APISIX_IT=1 go test ./internal/apisix/... -run Integration -count=1 -v
tidy:        ; go mod tidy
