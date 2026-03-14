# Deployment

## Docker Compose (local dev)

Starts the app, PostgreSQL, Kafka, and Zookeeper in one command.

```bash
cp .env.example .env          # set JWT_SECRET etc.
docker compose -f docker/docker-compose.yml up --build
```

Services exposed on localhost:

| Service    | Port  |
|------------|-------|
| App (API)  | 8080  |
| PostgreSQL | 5432  |
| Kafka      | 9092  |
| Zookeeper  | 2181  |

The app container connects to Kafka via the internal Docker network (`kafka:29092`).

## Dockerfile

Multi-stage build in `docker/Dockerfile`:

1. **Builder** — `golang:1.22-alpine` compiles the binary with `CGO_ENABLED=0`.
2. **Runtime** — `gcr.io/distroless/static-debian12` — minimal, no shell, no package manager.

The `migrations/` directory is copied into the image so the server can run them on startup.

## Kubernetes

Manifests in `k8s/deployment.yaml`:

- **Deployment** — 2 replicas minimum, rolling update strategy.
- **Service** — ClusterIP on port 80 → 8080.
- **HPA** — scales 2→10 replicas at 60% CPU utilization.
- **ConfigMap** — non-secret env vars.
- **Secrets** — `DATABASE_DSN` and `JWT_SECRET` must be created manually:

```bash
kubectl create secret generic cloudtalk-secrets \
  --from-literal=DATABASE_DSN="postgres://user:pass@host:5432/cloudtalk" \
  --from-literal=JWT_SECRET="your-secret-here"

kubectl apply -f k8s/deployment.yaml
```

### Kafka Consumer Groups in K8s

Each pod sets `KAFKA_GROUP_ID` to its own pod name (via Kubernetes Downward API). This ensures every pod gets its own copy of every Kafka message, enabling local WebSocket fan-out without a shared in-memory state.

### Health Probes

- liveness should hit `GET /health`
- readiness should hit `GET /ready`

Readiness now depends on:

- database connectivity
- Kafka producer connectivity
- Kafka consumer session readiness

### Production Config Checklist

Before deploying with `APP_ENV=prod`, ensure:

- `JWT_SECRET` is a strong random value with at least 32 characters
- `ALLOWED_ORIGINS` is explicitly set
- `DATABASE_DSN` does not point to localhost
- `KAFKA_BROKERS` does not use localhost-only endpoints
