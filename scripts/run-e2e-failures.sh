#!/usr/bin/env sh
set -eu

[ -f .env.dev ] || { printf '%s\n' 'Falta .env.dev. Copie .env.example a .env.dev y configure DB_PASSWORD local.' >&2; exit 1; }

docker compose --env-file .env.dev up -d postgres rabbitmq
export DB_PORT=5434
go run ./cmd/migrate
EVENTFLOW_E2E=1 go test -count=1 -tags e2e -v ./tests/e2e
