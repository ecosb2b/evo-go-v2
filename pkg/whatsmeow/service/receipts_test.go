package whatsmeow_service

import (
	"testing"
	"time"

	receipt_model "github.com/evolution-foundation/evolution-go/pkg/receipt/model"
	"go.mau.fi/whatsmeow/types"
)

func TestClassifyReceipt(t *testing.T) {
	cases := []struct {
		in      types.ReceiptType
		webhook string
		record  receipt_model.State
		known   bool
	}{
		{types.ReceiptTypeDelivered, "Delivered", receipt_model.StateDelivered, true},
		{types.ReceiptTypeRead, "Read", receipt_model.StateRead, true},
		{types.ReceiptTypePlayed, "Played", receipt_model.StatePlayed, true},
		{types.ReceiptTypeReadSelf, "ReadSelf", "", true},
		{types.ReceiptTypePlayedSelf, "PlayedSelf", "", true},
		{types.ReceiptTypeRetry, "", "", false},
		{types.ReceiptTypeSender, "", "", false},
		{types.ReceiptTypeServerError, "", "", false},
	}
	for _, c := range cases {
		webhook, record, known := classifyReceipt(c.in)
		if webhook != c.webhook || record != c.record || known != c.known {
			t.Errorf("classifyReceipt(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.in, webhook, record, known, c.webhook, c.record, c.known)
		}
	}
}

func TestReceiptChat(t *testing.T) {
	cases := map[types.JID]string{
		types.NewJID("5588997847126", types.DefaultUserServer):     "5588997847126@s.whatsapp.net",
		types.NewADJID("5588997847126", 0, 12):                     "5588997847126@s.whatsapp.net",
		types.NewJID("123456789", types.HiddenUserServer):          "123456789@lid",
		types.NewJID("120363000000000000", types.GroupServer):      "",
		types.NewJID("status", types.BroadcastServer):              "",
		types.NewJID("120363000000000001", types.NewsletterServer): "",
	}
	for in, want := range cases {
		if got := receiptChat(in); got != want {
			t.Errorf("receiptChat(%s) = %q, want %q", in, got, want)
		}
	}
}

type fakeReceiptRepo struct {
	calls chan []string
}

func (f *fakeReceiptRepo) Record(_ string, ids []string, _ string, _ receipt_model.State, _ time.Time) error {
	f.calls <- ids
	return nil
}
func (f *fakeReceiptRepo) Find(string, []string) ([]receipt_model.MessageReceipt, error) {
	return nil, nil
}
func (f *fakeReceiptRepo) DeleteOlderThan(time.Time) (int64, error) { return 0, nil }

// Grupo não é gravado; conversa individual é, com uma cópia dos ids (o
// chamador pode reaproveitar o slice).
func TestRecordReceiptSkipsGroups(t *testing.T) {
	repo := &fakeReceiptRepo{calls: make(chan []string, 2)}
	w := &whatsmeowService{receiptRepository: repo}

	w.RecordReceipt("inst", []string{"A"}, types.NewJID("120363000000000000", types.GroupServer), receipt_model.StateRead, time.Now())
	ids := []string{"B", "C"}
	w.RecordReceipt("inst", ids, types.NewJID("5588997847126", types.DefaultUserServer), receipt_model.StateRead, time.Now())
	ids[0] = "mudou"

	select {
	case got := <-repo.calls:
		if len(got) != 2 || got[0] != "B" || got[1] != "C" {
			t.Fatalf("ids gravados = %v, want [B C]", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("conversa individual não foi gravada")
	}
	select {
	case got := <-repo.calls:
		t.Fatalf("grupo foi gravado: %v", got)
	case <-time.After(200 * time.Millisecond):
	}
}
