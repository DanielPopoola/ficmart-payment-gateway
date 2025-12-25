package persistence

import (
	"context"
	"errors"
	"log/slog"

	"github.com/DanielPopoola/ficmart-payment-gateway/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Executor defines the common interface for pgxpool.Pool and pgx.Tx.
// This allows repositories to work seamlessly with both standalone connections and transactions.
type Executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type DB struct {
	Pool   *pgxpool.Pool
	logger *slog.Logger
}

// Connect establishes a connection to the PostgreSQL database using the provided configuration.
// It creates a connection pool with the specified settings and verifies connectivity by  the database.
func Connect(ctx context.Context, cfg *config.DatabaseConfig, logger *slog.Logger) (*DB, error) {
	pgxCfg, err := cfg.PgxConfig(ctx)
	if err != nil {
		logger.Error("failed to build pgx config", "error", err)
		return nil, err
	}

	logger.Info("connecting to database",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Name,
	)

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		logger.Error("failed to create connection pool", "error", err)
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		pool.Close()
		return nil, err
	}

	logger.Info("successfully connected to database",
		"max_conns", pgxCfg.MaxConns,
		"min_conns", pgxCfg.MinConns,
	)

	return &DB{
		Pool:   pool,
		logger: logger,
	}, nil
}

func (db *DB) Close() {
	db.logger.Info("closing database connection pool")
	db.Pool.Close()
}

// IsUniqueViolation checks if the given error is a PostgreSQL unique constraint violation.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
