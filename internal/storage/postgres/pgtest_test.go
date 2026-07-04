package postgres_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================================
// WARNING: NEVER point LEDGERLY_TEST_DSN at a database you care about.
// This harness CREATES and DROPS schemas (DROP SCHEMA ... CASCADE) in
// whatever database the DSN targets. Use a disposable container, e.g.:
//
//	docker run --rm -d -p 5432:5432 -e POSTGRES_PASSWORD=pw postgres:17-alpine
//
// ============================================================================

// setupTestSchema creates a randomly named, schema.sql-applied Postgres
// schema for a test, skipping unless LEDGERLY_TEST_DSN is set. It returns
// the DSN and the schema name; callers open pools against it with
// openTestPool. The schema (and everything in it) is dropped in
// t.Cleanup, regardless of how many separate pools were opened against it
// — this is what lets TestRestartSurvival close and reopen a pool against
// the same schema to simulate a service restart.
func setupTestSchema(t *testing.T) (dsn, schemaName string) {
	t.Helper()

	dsn = os.Getenv("LEDGERLY_TEST_DSN")
	if dsn == "" {
		t.Skip("LEDGERLY_TEST_DSN not set; skipping postgres integration test")
	}

	schemaName = "test_" + randomHex(t, 8)
	ctx := context.Background()

	setup := openTestPool(t, dsn, schemaName)
	defer setup.Close()

	if _, err := setup.Exec(ctx, "CREATE SCHEMA "+schemaName); err != nil {
		t.Fatalf("creating schema %s: %v", schemaName, err)
	}

	ddl, err := os.ReadFile(schemaSQLPath(t))
	if err != nil {
		t.Fatalf("reading db/schema.sql: %v", err)
	}
	if _, err := setup.Exec(ctx, string(ddl)); err != nil {
		t.Fatalf("applying schema.sql to schema %s: %v", schemaName, err)
	}

	t.Cleanup(func() {
		cleanup := openTestPool(t, dsn, schemaName)
		defer cleanup.Close()
		if _, err := cleanup.Exec(context.Background(), "DROP SCHEMA "+schemaName+" CASCADE"); err != nil {
			t.Logf("dropping schema %s: %v", schemaName, err)
		}
	})

	return dsn, schemaName
}

// openTestPool opens a pool whose connections default search_path to
// schemaName. schemaName always comes from randomHex (hex digits only), so
// the direct string concatenation into DDL statements in this file is not
// a SQL-injection risk.
func openTestPool(t *testing.T, dsn, schemaName string) *pgxpool.Pool {
	t.Helper()

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing LEDGERLY_TEST_DSN: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connecting to test postgres: %v", err)
	}
	return pool
}

// newTestPool returns a ready-to-use pool against a fresh, schema.sql-applied
// ephemeral schema, closed and dropped in t.Cleanup. Skips the test unless
// LEDGERLY_TEST_DSN is set.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn, schemaName := setupTestSchema(t)
	pool := openTestPool(t, dsn, schemaName)
	t.Cleanup(pool.Close)
	return pool
}

func randomHex(t *testing.T, n int) string {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating random schema suffix: %v", err)
	}
	return hex.EncodeToString(b)
}

// schemaSQLPath locates db/schema.sql relative to this test file, so tests
// work regardless of the working directory `go test` is invoked from.
func schemaSQLPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("determining db/schema.sql path: runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "db", "schema.sql")
}
