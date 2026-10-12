package user_service

import (
	"encoding/json"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestNewBlocklistResponseKeepsLegacyShape(t *testing.T) {
	pn := types.NewJID("5531987654321", types.DefaultUserServer)
	lidOnly := types.NewJID("123456789", types.HiddenUserServer)

	resp := newBlocklistResponse(&types.Blocklist{
		DHash: "abc",
		Items: []types.BlocklistItem{
			{LID: types.NewJID("987654321", types.HiddenUserServer), PN: pn, Active: true},
			{LID: lidOnly, Active: true},
		},
	})

	if resp.DHash != "abc" {
		t.Fatalf("DHash = %q, want abc", resp.DHash)
	}
	if len(resp.JIDs) != 2 || resp.JIDs[0] != pn || resp.JIDs[1] != lidOnly {
		t.Fatalf("JIDs = %v, want [%s %s]", resp.JIDs, pn, lidOnly)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DHash", "JIDs", "Items"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("JSON missing %q: %s", key, raw)
		}
	}
}

func TestNewBlocklistResponseNil(t *testing.T) {
	if newBlocklistResponse(nil) != nil {
		t.Fatal("want nil for nil blocklist")
	}
}
