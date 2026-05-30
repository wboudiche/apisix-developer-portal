.PHONY: up down run test tidy
up:          ; docker compose up -d
down:        ; docker compose down
run:         ; go run ./cmd/portal
test:        ; go test ./internal/... ./cmd/...
tidy:        ; go mod tidy
