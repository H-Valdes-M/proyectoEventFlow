$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath '.env.dev')) {
    throw 'Falta .env.dev. Copie .env.example a .env.dev y configure DB_PASSWORD local.'
}

docker compose --env-file .env.dev up -d postgres rabbitmq
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$env:DB_PORT = '5434'
go run ./cmd/migrate
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
$env:EVENTFLOW_E2E = '1'
go test -count=1 -tags e2e -v ./tests/e2e
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
