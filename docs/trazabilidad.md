# Trazabilidad de requisitos y evidencias

Esta tabla vincula los criterios del desafío con la implementación y las pruebas disponibles. Los tests de integración requieren servicios reales y se omiten si no se activan sus variables. El 14 de octubre de 2026 se repitieron correctamente las cuatro demostraciones E2E y las suites de integración PostgreSQL/RabbitMQ; los comandos siguientes permiten reproducirlas.

| ID | Estado | Implementación / evidencia automatizada |
| --- | --- | --- |
| RF-01 | Cubierto | `POST /orders`; `tests/integration/api_level1_integration_test.go`: `TestAPINivel1PersisteTrasReinicio`. Devuelve 201, ID y estado inicial. |
| RF-02 | Cubierto | Decoder JSON estricto y reglas de dominio. `internal/order/presentation/handler/order_handler_test.go`, `internal/order/domain/entity/*_test.go`; rechazo previo a persistencia. |
| RF-03 | Cubierto | `GET /orders/{id}`; tests HTTP del handler y persistencia; respuesta 404 para pedido inexistente. |
| RF-04 | Cubierto | Migraciones versionadas en `migrations/`; `TestAPINivel1PersisteTrasReinicio` y recreación Docker con volúmenes persistentes. |
| RF-05 | Cubierto | `pkg/httputil` y middleware traducen errores a respuestas uniformes; `cmd/routes/register_test.go` comprueba errores sanitizados. |
| RF-06 | Cubierto | Configuración `DB_*`, `PORT`, `ENVIRONMENT` y `.env.example`; pruebas en `cmd/config/`. `.env.dev` es local e ignorado. |
| RF-07 | Cubierto | Pruebas unitarias de dominio/casos de uso y HTTP con `httptest`; suites de integración en `tests/integration/`. |
| RF-08 | Cubierto | `compose.yaml`, `Dockerfile`, `cmd/migrate` y pasos reproducibles en el README. |
| RF-09 | Cubierto | Envelope con ID, tipo, fecha, versión y referencia al pedido; guardado en outbox en la transacción de creación. Pruebas en `internal/order/application/event/` e integración PostgreSQL/RabbitMQ. |
| RF-10 | Cubierto | Persistencia por clave y fingerprint; `TestAPIIdempotenciaConcurrente` cubre 20 POST concurrentes y conflicto de contenido. El E2E de respuesta perdida comprueba el replay. |
| RF-11 | Cubierto | Reintentos durables 1/2/4 s, máximo de cuatro resultados y fallo terminal persistido; `TestE2ECuatroFallosTransitoriosReinicioYDuplicadoTerminan`. |
| RF-12 | Cubierto | ACK manual después del commit; duplicados completados se ACKean, resultados no durables dejan la entrega sin confirmar. Pruebas en `tests/integration/` y E2E caída antes de ACK. |
| RF-13 | Cubierto | Zap JSON, `X-Request-ID`, correlación en evento/logs, métricas de cardinalidad acotada. Pruebas en `cmd/routes/` y `pkg/metrics/`. |
| RF-14 | Cubierto | Manejo SIGINT/SIGTERM, readiness no lista, drenaje con timeout y cierre de recursos; `cmd/app/shutdown_test.go` y `cmd/app/worker_test.go`. |

## Ejecución

### Unitarias, HTTP y análisis estático

```sh
gofmt -l .
go build ./...
go vet ./...
go test ./...
```

### Integración con PostgreSQL y RabbitMQ de Compose

Desde el host, la configuración `.env.dev` debe indicar `DB_PORT=5434` (el valor nuevo de `.env.example`). Levante primero los servicios siguiendo el README.

PowerShell:

```powershell
$env:EVENTFLOW_PG_INTEGRATION = '1'
$env:EVENTFLOW_MESSAGING_INTEGRATION = '1'
$env:DB_PORT = '5434'
go test -count=1 ./internal/order/infrastructure/postgres ./internal/health/infrastructure/postgres ./pkg/migrate ./tests/integration
```

Bash:

```sh
DB_PORT=5434 EVENTFLOW_PG_INTEGRATION=1 EVENTFLOW_MESSAGING_INTEGRATION=1 go test -count=1 ./internal/order/infrastructure/postgres ./internal/health/infrastructure/postgres ./pkg/migrate ./tests/integration
```

### Demostraciones E2E

```powershell
.\scripts\run-e2e-failures.ps1
```

```sh
sh ./scripts/run-e2e-failures.sh
```

Los escenarios y resultados esperados están descritos en [demo-fallos.md](demo-fallos.md). `go test -race ./...` es una comprobación adicional; necesita CGO y un compilador C disponibles.
