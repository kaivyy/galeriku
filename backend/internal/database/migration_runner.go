package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureMigrationTable ensures the schema_migrations tracking table exists.
func EnsureMigrationTable(ctx context.Context, pool *pgxpool.Pool) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version VARCHAR(64) PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
		);
	`
	_, err := pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}
	return nil
}

// GetAppliedMigrations returns a map of already applied migration versions.
func GetAppliedMigrations(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	if err := EnsureMigrationTable(ctx, pool); err != nil {
		return nil, err
	}

	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to query schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("failed to scan migration version: %w", err)
		}
		applied[v] = true
	}
	return applied, nil
}

// ApplyMigrationFile executes an individual SQL migration file within a transaction.
func ApplyMigrationFile(ctx context.Context, pool *pgxpool.Pool, filePath string) error {
	baseName := filepath.Base(filePath)
	version := strings.Split(baseName, "_")[0]

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", filePath, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for migration %s: %w", baseName, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("failed to execute migration %s: %w", baseName, err)
	}

	recordQuery := "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT (version) DO NOTHING"
	if _, err := tx.Exec(ctx, recordQuery, version); err != nil {
		return fmt.Errorf("failed to record migration %s in schema_migrations: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction for migration %s: %w", baseName, err)
	}

	log.Printf("[MIGRATOR] Successfully applied migration: %s", baseName)
	return nil
}

// RunPendingMigrations finds all .up.sql files in migrationsDir and applies unapplied ones in order.
func RunPendingMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) (int, error) {
	applied, err := GetAppliedMigrations(ctx, pool)
	if err != nil {
		return 0, err
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read migrations directory %s: %w", migrationsDir, err)
	}

	var upFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			upFiles = append(upFiles, entry.Name())
		}
	}
	sort.Strings(upFiles)

	appliedCount := 0
	for _, file := range upFiles {
		version := strings.Split(file, "_")[0]
		if applied[version] {
			continue
		}

		fullPath := filepath.Join(migrationsDir, file)
		if err := ApplyMigrationFile(ctx, pool, fullPath); err != nil {
			return appliedCount, err
		}
		appliedCount++
	}

	return appliedCount, nil
}

// RollbackMigrationFile executes an individual down SQL migration file within a transaction
// and removes its version from the schema_migrations table.
func RollbackMigrationFile(ctx context.Context, pool *pgxpool.Pool, filePath string) error {
	baseName := filepath.Base(filePath)
	version := strings.Split(baseName, "_")[0]

	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read rollback migration file %s: %w", filePath, err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for rollback %s: %w", baseName, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, string(content)); err != nil {
		return fmt.Errorf("failed to execute rollback migration %s: %w", baseName, err)
	}

	deleteQuery := "DELETE FROM schema_migrations WHERE version = $1"
	if _, err := tx.Exec(ctx, deleteQuery, version); err != nil {
		return fmt.Errorf("failed to delete migration %s from schema_migrations: %w", version, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction for rollback %s: %w", baseName, err)
	}

	log.Printf("[MIGRATOR] Successfully rolled back migration: %s", baseName)
	return nil
}

// RollbackLastMigration finds the latest applied migration in schema_migrations
// and executes its corresponding .down.sql file.
func RollbackLastMigration(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) (string, error) {
	if err := EnsureMigrationTable(ctx, pool); err != nil {
		return "", err
	}

	var latestVersion string
	err := pool.QueryRow(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&latestVersion)
	if err != nil {
		return "", fmt.Errorf("no applied migrations to rollback: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return "", fmt.Errorf("failed to read migrations directory %s: %w", migrationsDir, err)
	}

	var downFile string
	prefix := latestVersion + "_"
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".down.sql") {
			downFile = entry.Name()
			break
		}
	}

	if downFile == "" {
		return "", fmt.Errorf("no matching .down.sql found for version %s", latestVersion)
	}

	fullPath := filepath.Join(migrationsDir, downFile)
	if err := RollbackMigrationFile(ctx, pool, fullPath); err != nil {
		return "", err
	}

	return latestVersion, nil
}

