package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RunMigrations reads all SQL files in the migrations folder (or a single file if it's not a dir) and executes them.
func RunMigrations(ctx context.Context, db *sql.DB, path string) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get database connection for migrations: %w", err)
	}
	defer conn.Close()

	// Acquire a session-level advisory lock using a unique key to prevent multi-node migration deadlocks
	const migrationLockKey = 543210
	_, err = conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey)
	if err != nil {
		return fmt.Errorf("failed to acquire migration advisory lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat migration path: %w", err)
	}

	if !info.IsDir() {
		return runSingleMigration(ctx, conn, path)
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(strings.ToLower(f.Name()), ".sql") {
			sqlFiles = append(sqlFiles, filepath.Join(path, f.Name()))
		}
	}

	sort.Strings(sqlFiles)

	for _, file := range sqlFiles {
		if err := runSingleMigration(ctx, conn, file); err != nil {
			return fmt.Errorf("migration failed in file %s: %w", filepath.Base(file), err)
		}
	}

	return nil
}

func runSingleMigration(ctx context.Context, conn *sql.Conn, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", filePath, err)
	}

	_, err = conn.ExecContext(ctx, string(data))
	if err != nil {
		return fmt.Errorf("failed to execute migration %s: %w", filePath, err)
	}

	return nil
}
