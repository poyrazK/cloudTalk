# Architecture

## Overview

cloudTalk is a horizontally-scalable chat backend. Multiple server instances can run in parallel — users connected to different instances still receive each other's messages through Apache Kafka.

## Data Flow

```
Client A (WS) ──► Server Instance 1
                       │
                  Local Hub           ──► Kafka Producer ──► [chat.room.messages]
                  (fan-out to                                         │
                   local clients)                                     │
                                      ◄── Kafka Consumer ◄───────────┘
Client B (WS) ──► Server Instance 2
                       │
                  Local Hub           ──► Kafka Producer ──► [chat.dm.messages]
                  (fan-out to
                   local clients)
```

**Step by step:**
1. Client sends a message over WebSocket to whichever instance it is connected to.
2. The instance persists the message to PostgreSQL and publishes a `ChatEvent` to Kafka.
3. Every running instance consumes from Kafka (each pod has a unique consumer group ID).
4. Each instance fans out the event to any locally-connected WebSocket clients in the same room or as the DM recipient.

## Components

### Hub (`internal/hub/`)
In-memory registry of active WebSocket connections on a single instance. Supports room-based broadcast and direct user delivery. Thread-safe.

### Kafka Producer/Consumer (`internal/kafka/`)
Wraps IBM/sarama. The producer publishes `ChatEvent` JSON payloads. The consumer group uses the pod hostname as its group ID so every pod gets its own copy of each event.

Startup verifies required Kafka topics before serving traffic. Runtime readiness depends on both producer connectivity and consumer session health.

### Auth Service (`internal/auth/`)
Stateless JWT access tokens (15 min) + opaque refresh tokens (7 days, stored hashed in DB). No shared session store needed across instances.

### Repository Layer (`internal/repository/`)
All database access via pgx/v5. Cursor-based pagination for message history (uses `created_at < :before` rather than OFFSET).

### Service Layer (`internal/service/`)
Orchestrates business logic: room membership checks before sending, saving to DB and publishing to Kafka atomically from the caller's perspective.

For message-level behavior details (receipts, unread counts, edit/delete), see `docs/messaging.md`.

## Observability

- `/health` is a liveness endpoint for basic process availability.
- `/ready` verifies runtime readiness, including database connectivity.
- `/metrics` exposes Prometheus metrics for HTTP, WebSocket, Kafka, and DB pool activity.
- HTTP requests carry request IDs and are logged with route pattern, status, and latency.
- OpenTelemetry tracing can be enabled with OTLP export for HTTP, Kafka, WebSocket, and service flows.
