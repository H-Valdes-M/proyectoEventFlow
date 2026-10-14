# Decisiones, trade-offs y límites conocidos

## Modularidad antes que microservicios

El dominio, la aplicación y los adaptadores están separados por módulos verticales (`internal/order` e `internal/health`). API y worker son procesos separados porque tienen responsabilidades y dependencias distintas: la API no conecta a RabbitMQ. Se mantienen en el mismo repositorio y comparten contratos para evitar complejidad de despliegue prematura.

## PostgreSQL y transactional outbox

La creación guarda el pedido, los artículos, el resultado idempotente y el evento outbox en una transacción. Así, la confirmación de PostgreSQL no puede dejar el evento fuera del almacenamiento persistente, aunque RabbitMQ esté caído. El relay publica con confirmación y solo marca la fila publicada después del confirm positivo.

El relay y RabbitMQ ofrecen entrega **al menos una vez**, no exactamente una vez: una caída después de publicar y antes de marcar el outbox puede provocar una publicación duplicada. El consumidor deduplica por `(consumer_name, event_id)` en la misma transacción que persiste el efecto simulado. No hay coordinación/lease implementada para escalar el relay a múltiples réplicas; Compose ejecuta una instancia.

## Idempotencia HTTP

`POST /orders` requiere `Idempotency-Key`. Se guarda una huella canónica de la operación y un snapshot de la respuesta inicial. Repetir una solicitud equivalente devuelve el mismo resultado; usar la clave con datos distintos responde `409`. Las claves no tienen expiración automática en esta versión, por lo que su retención crece con el uso.

## Reintentos y confirmación

El consumidor procesa con ACK manual. Confirma después de completar durablemente el trabajo o reconocer un duplicado completado. Los fallos transitorios generan continuaciones persistidas con demoras de 1, 2 y 4 segundos, hasta cuatro resultados de negocio. Después registra el fallo terminal en PostgreSQL; las entregas no se eliminan sin que la decisión correspondiente haya quedado durable.

El texto crudo del error no se persiste en el registro de reintento: se guarda la descripción fija `fallo transitorio de procesamiento`. Esto evita que mensajes de errores con SQL, credenciales u otros detalles internos se conviertan en datos operativos persistidos. La investigación se basa en identificadores correlacionados, intento, estado y razón sanitizada.

## Dinero y validación

Los precios y totales se representan con `int64` en unidades menores de CLP (sin `float64`). El dominio valida cantidades/precios y rechaza overflow antes del cálculo. HTTP distingue errores de formato de JSON (`400`) de reglas de negocio (`422`), y traduce errores internos a respuestas sanitizadas.

## Observabilidad y operación

Se usan logs estructurados con identificadores de solicitud/evento y métricas locales de cardinalidad acotada. `/healthz` comprueba liveness y `/readyz` disponibilidad de PostgreSQL y migraciones. API y worker tienen apagado ordenado con un límite configurable. No se registran cuerpos, claves de idempotencia ni credenciales.

Compose publica puertos solo en loopback y usa credenciales de laboratorio; no son valores para producción. El endpoint `/metrics` no configura por sí mismo un backend de métricas externo.

## Límites y siguientes pasos

- La notificación es simulada; no hay correo ni pasarela de pagos reales.
- No se incluye Kubernetes, alta disponibilidad, despliegue cloud ni múltiples regiones.
- El consumidor actual es serial (`CONSUMER_PREFETCH=1`) y el relay se opera como una instancia.
- Las claves idempotentes se retienen indefinidamente.
- Para producción habría que definir retención/limpieza, observabilidad centralizada, estrategia de escalado con claim de outbox y políticas de recuperación operacional acordes al despliegue.

Estas limitaciones son deliberadas para mantener el alcance del desafío y hacer verificables las garantías implementadas.
