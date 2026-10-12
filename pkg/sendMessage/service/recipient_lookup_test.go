package send_service

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPickRegisteredJID(t *testing.T) {
	withNine := types.NewJID("5531987654321", types.DefaultUserServer)
	withoutNine := types.NewJID("553187654321", types.DefaultUserServer)
	candidates := []string{"+5531987654321", "+553187654321"}

	tests := []struct {
		name   string
		resp   []types.IsOnWhatsAppResponse
		want   types.JID
		wantOK bool
	}{
		{
			name: "only the form with the 9 is registered",
			resp: []types.IsOnWhatsAppResponse{
				{Query: "+553187654321", IsIn: false},
				{Query: "+5531987654321", JID: withNine, IsIn: true},
			},
			want: withNine, wantOK: true,
		},
		{
			name: "only the legacy form without the 9 is registered",
			resp: []types.IsOnWhatsAppResponse{
				{Query: "+5531987654321", IsIn: false},
				{Query: "+553187654321", JID: withoutNine, IsIn: true},
			},
			want: withoutNine, wantOK: true,
		},
		{
			name: "both registered: candidate order wins, not response order",
			resp: []types.IsOnWhatsAppResponse{
				{Query: "+553187654321", JID: withoutNine, IsIn: true},
				{Query: "+5531987654321", JID: withNine, IsIn: true},
			},
			want: withNine, wantOK: true,
		},
		{
			name: "query not echoed, single registered form",
			resp: []types.IsOnWhatsAppResponse{
				{JID: withoutNine, IsIn: true},
				{IsIn: false},
			},
			want: withoutNine, wantOK: true,
		},
		{
			name: "query not echoed, both forms resolve to the same JID",
			resp: []types.IsOnWhatsAppResponse{
				{JID: withoutNine, IsIn: true},
				{JID: withoutNine, IsIn: true},
			},
			want: withoutNine, wantOK: true,
		},
		{
			name: "none registered",
			resp: []types.IsOnWhatsAppResponse{
				{Query: "+5531987654321", IsIn: false},
				{Query: "+553187654321", IsIn: false},
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := pickRegisteredJID(candidates, tt.resp)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Fatalf("jid = %s, want %s", got, tt.want)
			}
		})
	}
}
