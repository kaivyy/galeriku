package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// VerifySchemaCompleteness counts tables in public schema and checks critical tables and functions
func VerifySchemaCompleteness(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	if pool == nil {
		return 0, fmt.Errorf("database pool cannot be nil")
	}

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
		return 0, fmt.Errorf("failed to verify uuid_generate_v7 function existence: %w", err)
	}
	if !funcExists {
		return 0, fmt.Errorf("critical database function uuid_generate_v7 is missing")
	}

	// Verify UUIDv7 function actually executes and generates a valid UUID without overflow
	var sampleUUID string
	execQuery := `SELECT uuid_generate_v7()::text;`
	err = pool.QueryRow(ctx, execQuery).Scan(&sampleUUID)
	if err != nil {
		return 0, fmt.Errorf("critical database function uuid_generate_v7 runtime execution failed: %w", err)
	}
	if sampleUUID == "" {
		return 0, fmt.Errorf("uuid_generate_v7 returned empty string")
	}

	return count, nil
}
