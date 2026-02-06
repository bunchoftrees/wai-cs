package db

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/workforce-ai/site-selection-iq/internal/config"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Pool is a type alias for pgxpool.Pool for use in other packages.
type Pool = pgxpool.Pool

// Connect creates a connection pool to Postgres.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse db config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxConns)
	poolCfg.MinConns = 2
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	slog.Info("connected to database",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.DBName,
		"max_conns", cfg.MaxConns,
	)

	return pool, nil
}

// RunMigrations executes embedded SQL migrations.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sqlBytes, err := migrations.ReadFile("migrations/001_initial_schema.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	_, err = pool.Exec(ctx, string(sqlBytes))
	if err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	slog.Info("database migrations applied successfully")
	return nil
}
