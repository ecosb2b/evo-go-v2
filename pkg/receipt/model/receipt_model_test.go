package receipt_model

import (
	"testing"
	"time"
)

func TestApplyAndStatus(t *testing.T) {
	t0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)

	r := &MessageReceipt{}
	if got := Status(r); got != StatusUnknown {
		t.Fatalf("vazio: status = %q, want unknown", got)
	}

	Apply(r, StateSent, t0)
	if got := Status(r); got != StatusSent {
		t.Fatalf("enviada: status = %q", got)
	}

	Apply(r, StateDelivered, t0.Add(time.Second))
	if got := Status(r); got != StatusDelivered {
		t.Fatalf("entregue: status = %q", got)
	}

	Apply(r, StateRead, t0.Add(time.Minute))
	if got := Status(r); got != StatusRead {
		t.Fatalf("lida: status = %q", got)
	}

	Apply(r, StatePlayed, t0.Add(2*time.Minute))
	if got := Status(r); got != StatusPlayed {
		t.Fatalf("reproduzida: status = %q", got)
	}
}

// Lida ou reproduzida sem o recibo de entrega: a entrega recebe a mesma data.
func TestReadImpliesDelivered(t *testing.T) {
	t0 := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	for _, state := range []State{StateRead, StatePlayed} {
		r := &MessageReceipt{}
		Apply(r, state, t0)
		if r.DeliveredAt == nil || !r.DeliveredAt.Equal(t0) {
			t.Errorf("%s: deliveredAt = %v, want %v", state, r.DeliveredAt, t0)
		}
	}
}

// Reproduzida não implica lida: o "lida" depende da confirmação de leitura do
// contato, a reprodução não.
func TestPlayedDoesNotImplyRead(t *testing.T) {
	r := &MessageReceipt{}
	Apply(r, StatePlayed, time.Now())
	if r.ReadAt != nil {
		t.Fatal("played preencheu readAt")
	}
}

func TestApplyStoresUTC(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	r := &MessageReceipt{}
	Apply(r, StateSent, time.Date(2026, 10, 17, 9, 0, 0, 0, loc))
	if r.SentAt.Location() != time.UTC || r.SentAt.Hour() != 12 {
		t.Fatalf("sentAt = %v, want 12:00 UTC", r.SentAt)
	}
}

func TestToView(t *testing.T) {
	v := ToView("ABC", nil)
	if v.MessageID != "ABC" || v.Status != StatusUnknown || v.SentAt != nil {
		t.Fatalf("nil: %+v", v)
	}

	r := &MessageReceipt{MessageID: "ABC", Chat: "5588997847126@s.whatsapp.net"}
	Apply(r, StateRead, time.Now())
	v = ToView("ABC", r)
	if v.Status != StatusRead || v.Chat != r.Chat || v.ReadAt == nil || v.DeliveredAt == nil {
		t.Fatalf("lida: %+v", v)
	}
}

func TestToViewReturnsUTC(t *testing.T) {
	brt := time.Date(2026, 10, 17, 9, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	r := &MessageReceipt{SentAt: &brt}
	v := ToView("ABC", r)
	if v.SentAt.Location() != time.UTC || v.SentAt.Hour() != 12 {
		t.Fatalf("sentAt = %v, want 12:00 UTC", v.SentAt)
	}
	if r.SentAt.Location() == time.UTC {
		t.Fatal("ToView alterou o registro original")
	}
}
