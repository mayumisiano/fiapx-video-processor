// Package postgrestest sets up a real Postgres database for integration
// tests (docs/technical-architecture.md §10): apply migrations, run the
// test, tear the schema back down. It is only ever imported from
// *_test.go files built with the "integration" tag.
package postgrestest

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Pool connects using DATABASE_URL, applies every migration found in
// migrationsDir, and returns a ready pool. It skips the test if
// DATABASE_URL is unset, so this only runs where a real database is
// available (CI's integration job, or a developer opting in locally).
func Pool(t *testing.T, migrationsDir string) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to postgres: %v", err)
	}
	t.Cleanup(pool.Close)

	applyMigrations(t, pool, migrationsDir)
	return pool
}

func applyMigrations(t *testing.T, pool *pgxpool.Pool, dir string) {
	t.Helper()

	ups, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	sort.Strings(ups)

	for _, f := range ups {
		runSQLFile(t, pool, f)
	}

	t.Cleanup(func() {
		downs, err := filepath.Glob(filepath.Join(dir, "*.down.sql"))
		if err != nil {
			return
		}
		sort.Sort(sort.Reverse(sort.StringSlice(downs)))
		for _, f := range downs {
			runSQLFile(t, pool, f)
		}
	})
}

func runSQLFile(t *testing.T, pool *pgxpool.Pool, path string) {
	t.Helper()

	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("exec %s: %v", path, err)
	}
}
