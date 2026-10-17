package receipt_repository

import (
	"os"
	"testing"
	"time"

	receipt_model "github.com/evolution-foundation/evolution-go/pkg/receipt/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Integração com Postgres (o upsert usa ON CONFLICT e LEAST). Roda só com
// RECEIPT_TEST_DSN definido, ex.:
//
//	RECEIPT_TEST_DSN=postgresql://u:p@localhost:5432/teste?sslmode=disable go test ./pkg/receipt/repository/
func openTestDB(t *testing.T) ReceiptRepository {
	dsn := os.Getenv("RECEIPT_TEST_DSN")
	if dsn == "" {
		t.Skip("RECEIPT_TEST_DSN não definido")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}
	if err := db.Migrator().DropTable(&receipt_model.MessageReceipt{}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := db.AutoMigrate(&receipt_model.MessageReceipt{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewReceiptRepository(db)
}

func find(t *testing.T, repo ReceiptRepository, instance, id string) *receipt_model.MessageReceipt {
	rows, err := repo.Find(instance, []string{id})
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("find %s/%s: %d linhas", instance, id, len(rows))
	}
	return &rows[0]
}

func TestRecordLifecycleAndOutOfOrder(t *testing.T) {
	repo := openTestDB(t)
	t0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	chat := "5588997847126@s.whatsapp.net"

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	must(repo.Record("inst-a", []string{"M1"}, chat, receipt_model.StateSent, t0))
	must(repo.Record("inst-a", []string{"M1"}, chat, receipt_model.StateRead, t0.Add(time.Minute)))
	// Entrega atrasada (chega depois da leitura, com data anterior): a data
	// mais antiga vence e o status continua "lida".
	must(repo.Record("inst-a", []string{"M1"}, chat, receipt_model.StateDelivered, t0.Add(10*time.Second)))
	// Leitura repetida mais tarde não muda a data da primeira.
	must(repo.Record("inst-a", []string{"M1"}, chat, receipt_model.StateRead, t0.Add(time.Hour)))

	r := find(t, repo, "inst-a", "M1")
	if got := receipt_model.Status(r); got != receipt_model.StatusRead {
		t.Fatalf("status = %q, want read", got)
	}
	if !r.SentAt.Equal(t0) {
		t.Errorf("sentAt = %v, want %v", r.SentAt, t0)
	}
	if !r.DeliveredAt.Equal(t0.Add(10 * time.Second)) {
		t.Errorf("deliveredAt = %v, want %v", r.DeliveredAt, t0.Add(10*time.Second))
	}
	if !r.ReadAt.Equal(t0.Add(time.Minute)) {
		t.Errorf("readAt = %v, want %v", r.ReadAt, t0.Add(time.Minute))
	}
	if r.PlayedAt != nil {
		t.Errorf("playedAt = %v, want nil", r.PlayedAt)
	}
	if r.Chat != chat {
		t.Errorf("chat = %q", r.Chat)
	}
}

func TestRecordBatchAndIsolationByInstance(t *testing.T) {
	repo := openTestDB(t)
	t0 := time.Now().UTC().Truncate(time.Millisecond)

	// Lote com repetido: grava cada id uma vez.
	if err := repo.Record("inst-a", []string{"A", "B", "A", ""}, "x@s.whatsapp.net", receipt_model.StateDelivered, t0); err != nil {
		t.Fatal(err)
	}
	// O mesmo id em outra instância é outra mensagem.
	if err := repo.Record("inst-b", []string{"A"}, "y@s.whatsapp.net", receipt_model.StatePlayed, t0); err != nil {
		t.Fatal(err)
	}

	rows, err := repo.Find("inst-a", []string{"A", "B", "C"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("inst-a: %d linhas, want 2", len(rows))
	}
	for _, r := range rows {
		if receipt_model.Status(&r) != receipt_model.StatusDelivered {
			t.Errorf("inst-a %s: status %q", r.MessageID, receipt_model.Status(&r))
		}
	}
	if s := receipt_model.Status(find(t, repo, "inst-b", "A")); s != receipt_model.StatusPlayed {
		t.Errorf("inst-b A: status %q, want played", s)
	}
}

func TestRecordKeepsFirstChat(t *testing.T) {
	repo := openTestDB(t)
	now := time.Now()
	_ = repo.Record("inst-a", []string{"M"}, "5588997847126@s.whatsapp.net", receipt_model.StateSent, now)
	_ = repo.Record("inst-a", []string{"M"}, "123@lid", receipt_model.StateRead, now)
	if r := find(t, repo, "inst-a", "M"); r.Chat != "5588997847126@s.whatsapp.net" {
		t.Errorf("chat = %q, want o do envio", r.Chat)
	}
}

func TestDeleteOlderThan(t *testing.T) {
	repo := openTestDB(t)
	_ = repo.Record("inst-a", []string{"OLD", "NEW"}, "x@s.whatsapp.net", receipt_model.StateSent, time.Now())
	n, err := repo.DeleteOlderThan(time.Now().Add(time.Hour)) // tudo é "antigo" para este corte
	if err != nil || n != 2 {
		t.Fatalf("DeleteOlderThan = (%d, %v), want (2, nil)", n, err)
	}
	n, _ = repo.DeleteOlderThan(time.Now().Add(-time.Hour))
	if n != 0 {
		t.Fatalf("segunda limpeza apagou %d", n)
	}
}
