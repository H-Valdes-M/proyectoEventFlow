# EventFlow — API de pedidos orientada a eventos

EventFlow recibe pedidos por HTTP, los persiste en PostgreSQL y procesa el trabajo posterior de forma asíncrona. La API guarda el pedido y el evento en una misma transacción mediante un **transactional outbox**; un worker publica en RabbitMQ y consume el evento con deduplicación, reintentos durables y ACK manual.

## Arquitectura y decisiones

- [Diagrama de arquitectura](docs/arquitectura.md)
- [Decisiones, trade-offs y límites conocidos](docs/decisiones.md)
- [Trazabilidad de RF-01 a RF-14 y comandos de verificación](docs/trazabilidad.md)
- [Demostraciones de fallos](docs/demo-fallos.md)

La solución usa dos procesos del mismo servicio (`cmd/api` y `cmd/worker`), no microservicios independientes. La API no necesita conectarse a RabbitMQ para aceptar pedidos.

## Requisitos

- Go 1.25.5 para ejecutar y probar desde el host.
- Docker Engine y Docker Compose para PostgreSQL y RabbitMQ locales.

No se necesitan cuentas cloud ni credenciales privadas. Los valores de `.env.example` son solo para el laboratorio local.

## Inicio local con Docker Compose

Desde la raíz de `proyectoEventFlow`, cree la configuración local:

Si ya existe `.env.dev`, conserve sus credenciales y confirme que `DB_PORT=5434` para la conexión desde el host a PostgreSQL de Compose.

**PowerShell**

```powershell
if (-not (Test-Path .env.dev)) { Copy-Item .env.example .env.dev }
docker compose --env-file .env.dev config
docker compose --env-file .env.dev up --build -d
Invoke-RestMethod http://127.0.0.1:8080/readyz
```

**Bash**

```sh
[ -f .env.dev ] || cp .env.example .env.dev
docker compose --env-file .env.dev config
docker compose --env-file .env.dev up --build -d
curl --fail http://127.0.0.1:8080/readyz
```

Compose aplica las migraciones antes de iniciar la API y el worker. Desde el host, PostgreSQL escucha en `127.0.0.1:5434` y RabbitMQ en `127.0.0.1:5672`; dentro de Compose los servicios usan `postgres:5432` y `rabbitmq:5672`. La API está en `127.0.0.1:8080` y la consola local de RabbitMQ en `127.0.0.1:15672`.

Para revisar el estado y los logs:

```sh
docker compose --env-file .env.dev ps
docker compose --env-file .env.dev logs api worker
```

Para detener el laboratorio conservando sus datos, ejecute `docker compose --env-file .env.dev down`. No use `down -v` salvo que quiera borrar explícitamente los volúmenes.

## Requests de ejemplo

La creación requiere `Idempotency-Key`. Si se repite la misma solicitud con la misma clave, la API devuelve el resultado original; reutilizar la clave con contenido distinto devuelve `409`.

**PowerShell**

```powershell
$body = @{
  customer_id = 'customer-123'
  items = @(@{
    product_id = 'book-001'
    quantity = 2
    unit_price_minor = 1500
  })
} | ConvertTo-Json -Depth 5

$headers = @{ 'Idempotency-Key' = 'demo-order-001' }
$order = Invoke-RestMethod -Uri http://127.0.0.1:8080/orders `
  -Method Post -Headers $headers -ContentType 'application/json' -Body $body
$order
Invoke-RestMethod "http://127.0.0.1:8080/orders/$($order.id)"
```

**Bash**

```sh
curl --fail-with-body -X POST http://127.0.0.1:8080/orders \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: demo-order-001' \
  -d '{"customer_id":"customer-123","items":[{"product_id":"book-001","quantity":2,"unit_price_minor":1500}]}'

# Sustituya <ORDER_ID> por el id devuelto por POST.
curl --fail-with-body http://127.0.0.1:8080/orders/<ORDER_ID>
```

El primer POST responde `201` con estado inicial `pending`. El worker procesa el evento y el GET refleja el estado actualizado, normalmente `processed`.

## Pruebas

Las pruebas unitarias y HTTP no necesitan servicios externos:

```sh
go test ./...
go vet ./...
```

Las pruebas de integración usan PostgreSQL y RabbitMQ reales. Con los servicios iniciados por Compose y `.env.dev` configurado:

**PowerShell**

```powershell
$env:EVENTFLOW_PG_INTEGRATION = '1'
$env:EVENTFLOW_MESSAGING_INTEGRATION = '1'
$env:DB_PORT = '5434'
go test -count=1 ./internal/order/infrastructure/postgres ./internal/health/infrastructure/postgres ./pkg/migrate ./tests/integration
```

**Bash**

```sh
DB_PORT=5434 EVENTFLOW_PG_INTEGRATION=1 EVENTFLOW_MESSAGING_INTEGRATION=1 go test -count=1 ./internal/order/infrastructure/postgres ./internal/health/infrastructure/postgres ./pkg/migrate ./tests/integration
```

Las pruebas de integración leen el resto de la configuración desde `.env.dev`; `DB_PORT=5434` selecciona el puerto publicado de PostgreSQL al ejecutarlas desde el host. La guía de [trazabilidad](docs/trazabilidad.md) contiene los comandos y evidencias por requisito.

Para ejecutar las cuatro demostraciones E2E reales:

```powershell
.\scripts\run-e2e-failures.ps1
```

```sh
sh ./scripts/run-e2e-failures.sh
```

El script de PowerShell y el de Bash levantan las dependencias, aplican migraciones y ejecutan los escenarios con el tag `e2e`. Requisitos, alcance y resultado esperado están en [demo-fallos](docs/demo-fallos.md).

## Diagnóstico rápido

- `GET /healthz`: liveness del proceso API.
- `GET /readyz`: readiness, que requiere PostgreSQL disponible y migraciones aplicadas.
- `GET /metrics`: métricas operativas en formato de texto.
- `GET /docs/index.html`: Swagger UI en entornos `dev` y `qa`.

## Limitaciones conocidas

El efecto posterior es una notificación simulada, no se envían correos reales ni se procesan pagos. El worker ejecuta el consumo serialmente con `CONSUMER_PREFETCH=1`; la solución está preparada para el laboratorio local, no promete alta disponibilidad ni coordinación de varios workers relay. La publicación usa entrega al menos una vez: una caída entre confirmación del broker y marcado del outbox puede duplicar el mensaje, que el consumidor deduplica en PostgreSQL.

Los siguientes pasos posibles están descritos junto con las decisiones y trade-offs en [docs/decisiones.md](docs/decisiones.md).
