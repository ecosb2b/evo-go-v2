package whatsmeow_service

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

// These tests cover the PostgreSQL connection leak that used to exhaust
// max_connections: StartClient built a new sqlstore.Container — and therefore a
// whole new *sql.DB pool — on every (re)connect, and nothing ever closed them.
//
// They exercise getAuthContainer, which memoises a single container in the
// package-level sharedAuthContainer. auth_container_retry_test.go covers the
// retry-after-failure behaviour; these tests add the part it does not measure:
// what the server actually sees in pg_stat_activity.
//
// They need a live PostgreSQL and are skipped unless POSTGRES_TEST_DSN is set,
// so ordinary `go test ./...` runs are unaffected:
//
//	POSTGRES_TEST_DSN='postgresql://postgres:root@localhost:5432/evogo_auth?sslmode=disable' go test ./pkg/whatsmeow/service/ -run AuthContainer -v

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	return dsn
}

// resetSharedAuthContainer gives a test a clean container and restores the
// previous one afterwards. sharedAuthContainer is process-wide, so without this
// the first test's container would carry into the others and the connection
// counts below would not measure what they claim to.
func resetSharedAuthContainer(t *testing.T) {
	t.Helper()

	sharedAuthContainerMu.Lock()
	previous := sharedAuthContainer
	sharedAuthContainer = nil
	sharedAuthContainerMu.Unlock()

	t.Cleanup(func() {
		sharedAuthContainerMu.Lock()
		defer sharedAuthContainerMu.Unlock()
		if sharedAuthContainer != nil && sharedAuthContainer != previous {
			_ = sharedAuthContainer.Close()
		}
		sharedAuthContainer = previous
	})
}

func newTestService(dsn string) *whatsmeowService {
	return &whatsmeowService{config: &config.Config{PostgresAuthDB: dsn}}
}

// openProbe returns a single-connection pool used only to observe the server's
// backend count, so it does not distort the measurement itself.
func openProbe(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	probe, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open probe: %v", err)
	}
	probe.SetMaxOpenConns(1)
	if err := probe.Ping(); err != nil {
		t.Fatalf("ping probe: %v", err)
	}
	return probe
}

// countBackends reports how many server-side connections exist for this
// database, excluding the probe's own backend.
func countBackends(t *testing.T, probe *sql.DB) int {
	t.Helper()
	var n int
	err := probe.QueryRow(
		`SELECT count(*) FROM pg_stat_activity
		 WHERE datname = current_database() AND pid <> pg_backend_pid()`,
	).Scan(&n)
	if err != nil {
		t.Fatalf("count backends: %v", err)
	}
	return n
}

// maxAuthOpenConns mirrors the Postgres cap set in getAuthContainer.
const maxAuthOpenConns = 20

// TestAuthContainerReusesSinglePool is the regression test for the fix: many
// StartClient-equivalent calls must share one container, and therefore one
// bounded pool.
func TestAuthContainerReusesSinglePool(t *testing.T) {
	dsn := testDSN(t)
	resetSharedAuthContainer(t)
	probe := openProbe(t, dsn)
	defer probe.Close()

	before := countBackends(t, probe)
	w := newTestService(dsn)

	first, err := w.getAuthContainer()
	if err != nil {
		t.Fatalf("getAuthContainer: %v", err)
	}

	const iterations = 60
	for i := 0; i < iterations; i++ {
		got, err := w.getAuthContainer()
		if err != nil {
			t.Fatalf("getAuthContainer #%d: %v", i, err)
		}
		if got != first {
			t.Fatalf("call #%d returned a different container: the pool is not being shared", i)
		}
	}

	after := countBackends(t, probe)
	t.Logf("FIXED: backends before=%d after=%d delta=%d over %d getAuthContainer() calls",
		before, after, after-before, iterations)

	if delta := after - before; delta > maxAuthOpenConns {
		t.Fatalf("backend count grew by %d, above the configured MaxOpenConns of %d", delta, maxAuthOpenConns)
	}
}

// TestAuthContainerConcurrentCallersShareOnePool guards the memoisation itself:
// every concurrent caller must observe the same container, so a burst of
// simultaneous (re)connects cannot open a second pool. Run with -race.
func TestAuthContainerConcurrentCallersShareOnePool(t *testing.T) {
	dsn := testDSN(t)
	resetSharedAuthContainer(t)

	w := newTestService(dsn)

	const goroutines = 32
	var wg sync.WaitGroup
	results := make([]*sqlstore.Container, goroutines)
	errs := make([]error, goroutines)

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = w.getAuthContainer()
		}(i)
	}
	wg.Wait()

	for i := range results {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if results[i] != results[0] {
			t.Fatalf("goroutine %d got a different container: the container is not shared", i)
		}
	}
	t.Logf("CONCURRENCY: all %d concurrent callers received the same container", goroutines)
}

// TestAuthContainerUnderLoadStaysBounded drives real query traffic through the
// shared container from many concurrent callers and samples the server-side
// backend count, showing connections are reused rather than accumulated.
func TestAuthContainerUnderLoadStaysBounded(t *testing.T) {
	dsn := testDSN(t)
	resetSharedAuthContainer(t)
	probe := openProbe(t, dsn)
	defer probe.Close()

	w := newTestService(dsn)
	before := countBackends(t, probe)

	const (
		workers          = 120
		queriesPerWorker = 10
	)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			c, err := w.getAuthContainer()
			if err != nil {
				return
			}
			for q := 0; q < queriesPerWorker; q++ {
				_, _ = c.GetAllDevices(context.Background())
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	// Sample the backend count from the test goroutine while the load runs.
	peak := before
sampling:
	for {
		select {
		case <-done:
			break sampling
		default:
			if n := countBackends(t, probe); n > peak {
				peak = n
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	after := countBackends(t, probe)
	t.Logf("LOAD: %d concurrent workers x %d queries — backends before=%d peak=%d after=%d",
		workers, queriesPerWorker, before, peak, after)

	if peak-before > maxAuthOpenConns {
		t.Fatalf("peak backend count grew by %d, above the configured MaxOpenConns of %d", peak-before, maxAuthOpenConns)
	}
}

// TestOldPerCallContainerLeaks reproduces the pre-fix behaviour to show the
// measurements above are actually sensitive to the bug, and to demonstrate the
// original failure mode. It is opt-in because it deliberately exhausts the
// server's connection slots: set EVO_LEAK_DEMO=1 to run it.
func TestOldPerCallContainerLeaks(t *testing.T) {
	if os.Getenv("EVO_LEAK_DEMO") != "1" {
		t.Skip("set EVO_LEAK_DEMO=1 to run the pre-fix leak demonstration")
	}
	dsn := testDSN(t)
	probe := openProbe(t, dsn)
	defer probe.Close()

	before := countBackends(t, probe)

	// Enough iterations to pass max_connections=100 if every container keeps its
	// own pool alive.
	const iterations = 150
	created := 0
	var failure error
	for i := 0; i < iterations; i++ {
		// Exactly what StartClient used to do: a fresh container per (re)connect,
		// never closed.
		if _, err := sqlstore.New(context.Background(), "postgres", dsn, nil); err != nil {
			failure = err
			t.Logf("OLD: sqlstore.New failed after %d containers: %v", created, err)
			break
		}
		created++
	}

	after := countBackends(t, probe)
	t.Logf("OLD: backends before=%d after=%d delta=%d over %d containers created",
		before, after, after-before, created)
	if failure == nil {
		t.Logf("OLD: reached %d containers without an error; server still had slots free", created)
	}
}
