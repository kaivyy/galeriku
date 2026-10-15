package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestVerifySchemaCompleteness(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5433"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = "galeriku"
		}
		pass := os.Getenv("DB_PASSWORD")
		if pass == "" {
			pass = "galeriku_secret_pass"
		}
		dbName := os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "galeriku_db"
		}
		sslMode := os.Getenv("DB_SSLMODE")
		if sslMode == "" {
			sslMode = "disable"
		}
		connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, dbName, sslMode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Skipf("PostgreSQL not accessible in test environment: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("PostgreSQL ping failed: %v", err)
	}

	count, err := VerifySchemaCompleteness(ctx, pool)
	if err != nil {
		t.Fatalf("VerifySchemaCompleteness failed: %v", err)
	}

	const expectedTables = 81
	if count < expectedTables {
		t.Fatalf("expected at least %d tables, got %d", expectedTables, count)
	}
	t.Logf("Successfully verified %d tables and uuid_generate_v7 in PostgreSQL schema", count)
}
