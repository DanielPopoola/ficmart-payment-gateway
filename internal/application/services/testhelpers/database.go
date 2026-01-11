package testhelpers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/DanielPopoola/ficmart-payment-gateway/internal/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDatabase struct {
	Container testcontainers.Container
	DB        *postgres.DB
	Config    *config.DatabaseConfig
}

func SetupTestDatabase(t *testing.T) *TestDatabase {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
			"POSTGRES_DB":       "testdb",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dbConfig := &config.DatabaseConfig{
		Host:            host,
		Port:            port.Int(),
		User:            "testuser",
		Password:        "testpass",
		Name:            "testdb",
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 1 * time.Hour,
		ConnMaxIdleTime: 30 * time.Minute,
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	db, err := postgres.Connect(ctx, dbConfig, logger)
	require.NoError(t, err)

	err = runMigrations(ctx, db)
	require.NoError(t, err)

	return &TestDatabase{
		Container: container,
		DB:        db,
		Config:    dbConfig,
	}
}

func (td *TestDatabase) Cleanup(t *testing.T) {
	ctx := context.Background()
	td.DB.Close()
	require.NoError(t, td.Container.Terminate(ctx))
}

func (td *TestDatabase) CleanTables(t *testing.T) {
	ctx := context.Background()

	_, err := td.DB.Pool.Exec(ctx, "TRUNCATE TABLE idempotency_keys, payments RESTART IDENTITY CASCADE;")
	require.NoError(t, err)
}

func getProjectRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
}

func runMigrations(ctx context.Context, db *postgres.DB) error {
	root := getProjectRoot()
	migrationPath := filepath.Join(root, "db", "migrations", "001_init.up.sql")

	migrationSQL, err := os.ReadFile(migrationPath) //nolint:gosec // test helper, controlled path
	if err != nil {
		return fmt.Errorf("read migration file from %s: %w", migrationPath, err)
	}

	_, err = db.Pool.Exec(ctx, string(migrationSQL))
	if err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	return nil
}
