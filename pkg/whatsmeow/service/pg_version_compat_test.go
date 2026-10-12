package whatsmeow_service

import (
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	label_model "github.com/evolution-foundation/evolution-go/pkg/label/model"
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
)

// TestPostgresVersionCompatibility exercises both schema-creation paths the
// service depends on, so a PostgreSQL major version can be qualified before
// anyone points production at it:
//
//   - the whatsmeow sqlstore migrations, which run through lib/pq
//   - GORM's AutoMigrate for the application models, which runs through pgx
//
// Point POSTGRES_TEST_DSN at the server to qualify and run it; skipped otherwise.
func TestPostgresVersionCompatibility(t *testing.T) {
	dsn := testDSN(t)

	probe := openProbe(t, dsn)
	defer probe.Close()

	var serverVersion, passwordEncryption string
	if err := probe.QueryRow("show server_version").Scan(&serverVersion); err != nil {
		t.Fatalf("read server_version: %v", err)
	}
	if err := probe.QueryRow("show password_encryption").Scan(&passwordEncryption); err != nil {
		t.Fatalf("read password_encryption: %v", err)
	}
	t.Logf("server_version=%s password_encryption=%s", serverVersion, passwordEncryption)

	// 1. whatsmeow store migrations, over lib/pq.
	resetSharedAuthContainer(t)
	w := newTestService(dsn)
	if _, err := w.getAuthContainer(); err != nil {
		t.Fatalf("whatsmeow sqlstore migrations over lib/pq: %v", err)
	}
	t.Log("whatsmeow sqlstore migrations (lib/pq): OK")

	// 2. Application models, over pgx.
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open over pgx: %v", err)
	}
	if err := gdb.AutoMigrate(
		&instance_model.Instance{},
		&message_model.Message{},
		&label_model.Label{},
	); err != nil {
		t.Fatalf("GORM AutoMigrate over pgx: %v", err)
	}
	t.Log("GORM AutoMigrate (pgx): OK")

	// Confirm the whatsmeow schema really landed rather than silently no-opping.
	var tables int
	if err := probe.QueryRow(
		`SELECT count(*) FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name LIKE 'whatsmeow_%'`,
	).Scan(&tables); err != nil {
		t.Fatalf("count whatsmeow tables: %v", err)
	}
	if tables == 0 {
		t.Fatal("no whatsmeow_* tables were created")
	}
	t.Logf("whatsmeow_* tables created: %d", tables)
}
