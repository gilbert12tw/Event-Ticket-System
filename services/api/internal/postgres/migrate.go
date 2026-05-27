package postgres

import (
	"context"
	_ "embed"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaMigrationLockID int64 = 811_204_202_601

var SchemaStatements = parseSchemaStatements(schemaSQL)

//go:embed schema.sql
var schemaSQL string

func parseSchemaStatements(schema string) []string {
	parts := strings.Split(schema, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		statement := strings.TrimSpace(part)
		if statement != "" {
			statements = append(statements, statement)
		}
	}
	return statements
}

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, schemaMigrationLockID); err != nil {
		return err
	}
	for _, statement := range SchemaStatements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
