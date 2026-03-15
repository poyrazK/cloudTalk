# Tracing

cloudTalk supports OpenTelemetry tracing over OTLP.

## Environment

- `TRACING_ENABLED=true`
- `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318/v1/traces`
- `OTEL_TRACES_SAMPLER_ARG=0.1`
- `OTEL_SERVICE_NAME=cloudtalk`
- `OTEL_SERVICE_VERSION=dev`

## Coverage

Tracing currently covers:

- incoming HTTP requests
- Kafka publish and consume operations
- WebSocket client message handling
- key service flows for DM conversations, room conversations, room members, and presence updates

Kafka trace context is propagated through message headers so publish and consume spans can be linked across instances.

## Local setup

Use any OTLP-compatible backend such as Jaeger, Grafana Tempo, or the OpenTelemetry Collector.

Example local OTLP HTTP endpoint:

```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318/v1/traces
```
