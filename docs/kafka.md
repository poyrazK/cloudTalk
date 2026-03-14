# Kafka

## Topics

| Topic                | Partition Key | Consumers       | Purpose                              |
|----------------------|---------------|-----------------|--------------------------------------|
| `chat.room.messages` | `room_id`     | All instances   | Room messages + typing indicators    |
| `chat.dm.messages`   | `receiver_id` | All instances   | Direct messages + DM typing signals  |
| `chat.presence`      | `user_id`     | All instances   | Online / offline presence events     |

cloudTalk verifies required topics at startup and fails fast if any are missing.

Required topics:

- `chat.room.messages`
- `chat.dm.messages`
- `chat.presence`

## ChatEvent Envelope

All Kafka messages share this JSON structure:

```json
{
  "type":        "message | dm | typing | typing_dm | presence",
  "room_id":     "<uuid>",
  "sender_id":   "<uuid>",
  "to_user_id":  "<uuid>",
  "payload":     { ...event-specific data }
}
```

`payload` contains the full serialized model object (Message, DirectMessage, etc.).

## Consumer Group Strategy

Each server instance uses a **unique consumer group ID** (default: `cloudtalk-<hostname>`).

This means every instance receives **every Kafka message** independently. Each instance then fans the event out only to clients that are locally connected and in the relevant room.

In Kubernetes, `KAFKA_GROUP_ID` is set to the pod name via the Downward API, guaranteeing uniqueness without coordination.

## Producer Config

- `RequiredAcks`: `WaitForLocal` — waits for the leader to acknowledge.
- `Compression`: Snappy — reduces bandwidth.
- `Return.Successes`: true — synchronous confirmation before continuing.

The readiness endpoint also performs a lightweight producer metadata refresh to verify Kafka connectivity.

## Consumer Health

Readiness includes consumer session health.

- before the first successful consumer group session, `/ready` returns `503`
- consumer loop errors mark Kafka consumer readiness unhealthy until the next successful session setup
- malformed Kafka messages are logged, counted, and marked consumed

## Failure Metrics

Kafka observability now includes metrics for:

- publish totals and latency
- consume totals and handler latency
- consumer loop errors
- message decode errors
- topic verification success/failure

## Fan-out Logic (`cmd/server/main.go → fanOut`)

| Topic                | Action |
|----------------------|--------|
| `chat.room.messages` | `typing` goes to subscribed room members except sender; other room events go to all subscribed room clients |
| `chat.dm.messages`   | `typing_dm` goes to receiver only; other DM events are echoed to both sides |
| `chat.presence`      | Handled by `PresenceService` |
