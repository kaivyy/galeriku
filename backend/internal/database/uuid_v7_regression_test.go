package database

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) (*pgxpool.Pool, context.Context, context.CancelFunc) {
	if testing.Short() {
		t.Skip("skipping live database test in short mode")
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

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		cancel()
		t.Skipf("PostgreSQL not accessible in test environment: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cancel()
		t.Skipf("PostgreSQL ping failed: %v", err)
	}

	return pool, ctx, cancel
}

// TestUUIDv7_Regression reproduces the defective UUIDv7 function behavior
func TestUUIDv7_Regression(t *testing.T) {
	pool, ctx, cancel := getTestPool(t)
	defer pool.Close()
	defer cancel()

	// 1. Direct execution test on uuid_generate_v7()
	var uuid1 string
	err := pool.QueryRow(ctx, "SELECT uuid_generate_v7()::text").Scan(&uuid1)
	if err != nil {
		t.Fatalf("uuid_generate_v7() execution failed: %v", err)
	}

	// 2. Direct execution test on alias generate_uuid_v7()
	var uuid2 string
	err = pool.QueryRow(ctx, "SELECT generate_uuid_v7()::text").Scan(&uuid2)
	if err != nil {
		t.Fatalf("generate_uuid_v7() execution failed: %v", err)
	}

	// 3. Bit layout verification for UUIDv7 (RFC 9562 / draft-peabody-dispatch-new-uuid-format)
	// Format: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx where y is 8, 9, a, or b
	uuid7Regex := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuid7Regex.MatchString(uuid1) {
		t.Errorf("uuid_generate_v7() produced invalid UUIDv7 bit layout: %s", uuid1)
	}
	if !uuid7Regex.MatchString(uuid2) {
		t.Errorf("generate_uuid_v7() produced invalid UUIDv7 bit layout: %s", uuid2)
	}

	// 4. Test insertion into representative table relying on DEFAULT generate_uuid_v7()
	var insertedID string
	testEmail := fmt.Sprintf("test_uuid_%d@galeriku.com", time.Now().UnixNano())
	query := `INSERT INTO users (email_normalized, account_status) VALUES ($1, 'ACTIVE') RETURNING id::text;`
	err = pool.QueryRow(ctx, query, testEmail).Scan(&insertedID)
	if err != nil {
		t.Fatalf("INSERT into users with default UUIDv7 failed: %v", err)
	}

	if !uuid7Regex.MatchString(insertedID) {
		t.Errorf("inserted user ID has invalid UUIDv7 bit layout: %s", insertedID)
	}

	// Cleanup test row
	_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email_normalized = $1", testEmail)
}
