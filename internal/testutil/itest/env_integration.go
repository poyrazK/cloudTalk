//go:build integration

package itest

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poyrazk/cloudtalk/internal/db"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

type EnvOptions struct {
	WithKafka bool
}

type Env struct {
	Postgres *tcpostgres.PostgresContainer
	Kafka    *tckafka.KafkaContainer
	Pool     *pgxpool.Pool
	DSN      string
	Brokers  []string
}

func Start(t *testing.T, opts EnvOptions) *Env {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	pg, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("cloudtalk"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(pg)
	})

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}

	if err := db.Migrate(dsn, "file://"+filepath.Join(repoRoot(), "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	pool, err := db.Connect(ctx, db.ConnectConfig{DSN: dsn, MaxConns: 10, MinConns: 1})
	if err != nil {
		t.Fatalf("connect pool: %v", err)
	}
	t.Cleanup(pool.Close)

	env := &Env{Postgres: pg, Pool: pool, DSN: dsn}

	if opts.WithKafka {
		k, err := tckafka.RunContainer(ctx)
		if err != nil {
			t.Fatalf("start kafka container: %v", err)
		}
		t.Cleanup(func() {
			_ = testcontainers.TerminateContainer(k)
		})
		brokers, err := k.Brokers(ctx)
		if err != nil {
			t.Fatalf("kafka brokers: %v", err)
		}
		env.Kafka = k
		env.Brokers = brokers
	}

	return env
}

func (e *Env) ResetDB(ctx context.Context) error {
	_, err := e.Pool.Exec(ctx, `
		TRUNCATE TABLE
			refresh_tokens,
			direct_messages,
			messages,
			room_members,
			rooms,
			users
		RESTART IDENTITY
		CASCADE
	`)
	if err != nil {
		return fmt.Errorf("truncate tables: %w", err)
	}
	return nil
}

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
