# cloudTalk

Distributed real-time chat backend written in Go.

## Quick Start

```bash
cp .env.example .env
docker compose -f docker/docker-compose.yml up --build
```

API: `http://localhost:8080` — WebSocket: `ws://localhost:8080/ws?token=<jwt>`

## Documentation

| Doc | Description |
|-----|-------------|
| [Architecture](docs/architecture.md) | System design & data flow |
| [API Reference](docs/api.md) | REST endpoints & request/response shapes |
| [WebSocket Protocol](docs/websocket.md) | WS message format for clients |
| [Auth](docs/auth.md) | JWT & refresh token details |
| [Kafka](docs/kafka.md) | Topics, events, consumer group strategy |
| [Database](docs/database.md) | Schema & migration guide |
| [Deployment](docs/deployment.md) | Docker Compose & Kubernetes |
| [Configuration](docs/configuration.md) | All environment variables |

## Tech Stack

Go · PostgreSQL · Apache Kafka · WebSocket · JWT · Docker · Kubernetes
