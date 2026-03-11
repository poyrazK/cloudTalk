# Kafka

## Topics

| Topic                | Partition Key | Consumers       | Purpose                              |
|----------------------|---------------|-----------------|--------------------------------------|
| `chat.room.messages` | `room_id`     | All instances   | Room messages + typing indicators    |
| `chat.dm.messages`   | `receiver_id` | All instances   | Direct messages + DM typing signals  |
| `chat.presence`      | `user_id`     | All instances   | Online / offline presence events     |

Topics are auto-created by Kafka on first publish (`KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`).

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

## Fan-out Logic (`cmd/server/main.go → fanOut`)

| Topic                | Action |
|----------------------|--------|
| `chat.room.messages` | `typing` goes to subscribed room members except sender; other room events go to all subscribed room clients |
| `chat.dm.messages`   | `typing_dm` goes to receiver only; other DM events are echoed to both sides |
| `chat.presence`      | Handled by `PresenceService` |
