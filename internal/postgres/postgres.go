// Package postgres opens the connection pool and reports its reachability.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool validates the connection string and returns a pool. Connections are
// established in the background, so this succeeds while the database is down
// and the readiness check is what reports it.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: %w", err)
	}
	return pool, nil
}

// Checker is the readiness check for Postgres.
type Checker struct {
	pool *pgxpool.Pool
}

func NewChecker(pool *pgxpool.Pool) Checker {
	return Checker{pool: pool}
}

func (Checker) Name() string { return "postgres" }

func (c Checker) Check(ctx context.Context) error {
	return c.pool.Ping(ctx)
}
