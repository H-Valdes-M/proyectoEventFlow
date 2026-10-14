# Demostraciones de fallos

Los escenarios E2E usan PostgreSQL y RabbitMQ reales. Deben ejecutarse desde la raíz del proyecto con Docker Compose disponible y `.env.dev` creado desde `.env.example`. Las pruebas llevan el tag `e2e`; los failpoints no quedan expuestos por HTTP ni se habilitan en el binario ordinario.

## Ejecutar todas las demostraciones

PowerShell:

```powershell
if (-not (Test-Path .env.dev)) { Copy-Item .env.example .env.dev }
.\scripts\run-e2e-failures.ps1
```

Bash:

```sh
[ -f .env.dev ] || cp .env.example .env.dev
sh ./scripts/run-e2e-failures.sh
```

Los scripts levantan PostgreSQL y RabbitMQ con la configuración local, usan el puerto PostgreSQL del host `5434`, aplican migraciones y corren `go test -count=1 -tags e2e -v ./tests/e2e`. Si ya existe `.env.dev`, no hace falta copiarlo de nuevo.

Para repetir un caso individual, primero levante `postgres` y `rabbitmq`, aplique migraciones (`DB_PORT=5434 go run ./cmd/migrate` en Bash o `$env:DB_PORT='5434'; go run ./cmd/migrate` en PowerShell), y active `EVENTFLOW_E2E=1` al ejecutar el test con `-tags e2e`.

## Escenarios

### Respuesta HTTP perdida después del commit — RF-10

`TestE2ERespuestaPerdidaReproduceSnapshot` detiene la respuesta después de confirmar la transacción y antes de escribir el HTTP response. El cliente vence su plazo y repite el POST con la misma `Idempotency-Key`. El replay recibe `201` con el snapshot original; queda un pedido y un evento inicial.

### RabbitMQ detenido — RF-04, RF-09

`TestE2EBrokerDetenidoSeRecuperaSinReenviarPOST` detiene RabbitMQ, crea un pedido y verifica que la API responde `201`, el pedido permanece `pending` y el evento sigue en outbox. Al iniciar el broker, el worker publica y procesa el evento sin repetir el POST.

### Caída después del commit del consumidor y antes del ACK — RF-12

`TestE2ECaidaEntreCommitYAckRedeliveryDeduplicada` interrumpe el worker tras confirmar en PostgreSQL la notificación, el estado del pedido y `event_processing.completed`, pero antes de `basic.ack`. RabbitMQ redelivery el evento a un worker reiniciado; la deduplicación evita una segunda notificación y permite confirmar el duplicado completado.

### Fallos transitorios, reinicio y agotamiento — RF-11

`TestE2ECuatroFallosTransitoriosReinicioYDuplicadoTerminan` inyecta fallos controlados en los intentos 1–4. Comprueba continuaciones durables con esperas 1, 2 y 4 segundos, reinicio durante la espera, duplicado sin una segunda cadena de reintentos y estado terminal `failed` con razón sanitizada en `failed_events`.

## Evidencia registrada

Estos cuatro escenarios se ejecutaron y aprobaron contra PostgreSQL y RabbitMQ reales el 14 de octubre de 2026 (17.477 s). Para volver a comprobarlos en otro entorno, use los comandos de esta página; las pruebas limpian sus datos identificados al finalizar.
