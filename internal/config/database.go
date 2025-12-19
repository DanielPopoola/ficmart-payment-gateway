package config

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgxConfig creates and returns a pgxpool.Config with the database connection settings from the DatabaseConfig.
func (c *DatabaseConfig) PgxConfig(ctx context.Context) (*pgxpool.Config, error) {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode,
	)

	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = int32(c.MaxOpenConns)
	cfg.MinConns = int32(c.MaxIdleConns)
	cfg.MaxConnLifetime = c.ConnMaxLifetime
	cfg.MaxConnIdleTime = c.ConnMaxIdleTime
	cfg.HealthCheckPeriod = 30 * time.Second

	return cfg, nil
}
