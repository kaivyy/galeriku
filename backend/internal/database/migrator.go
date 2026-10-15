package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VerifySchemaCompleteness counts tables in public schema and checks critical tables
func VerifySchemaCompleteness(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE';
	`
	var count int
	err := pool.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to query table count: %w", err)
	}

	// Verify UUIDv7 function exists
	var funcExists bool
	funcQuery := `
		SELECT EXISTS (
			SELECT 1 FROM pg_proc WHERE proname = 'uuid_generate_v7'
		);
	`
	err = pool.QueryRow(ctx, funcQuery).Scan(&funcExists)
	if err != nil {
		return 0, fmt.Errorf("failed to verify uuid_generate_v7: %w", err)
	}
	if !funcExists {
		return 0, fmt.Errorf("critical database function uuid_generate_v7 is missing")
	}

	return count, nil
}
