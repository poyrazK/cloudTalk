# Stage 1: build
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o cloudtalk ./cmd/server

# Stage 2: minimal runtime
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /app/cloudtalk .
COPY --from=builder /app/migrations ./migrations
EXPOSE 8080
ENTRYPOINT ["/app/cloudtalk"]
