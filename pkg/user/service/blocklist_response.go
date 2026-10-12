package user_service

import "go.mau.fi/whatsmeow/types"

// BlocklistResponse mantém o formato que /user/blocklist sempre devolveu
// ({"DHash", "JIDs"}) e acrescenta Items. O whatsmeow passou a separar LID e
// número em cada item; repassar o tipo dele mudaria o JSON para os clientes.
type BlocklistResponse struct {
	DHash string
	// JIDs: um por contato bloqueado, o número (PN) quando o WhatsApp informa
	// e o LID quando não.
	JIDs  []types.JID
	Items []types.BlocklistItem
}

func newBlocklistResponse(bl *types.Blocklist) *BlocklistResponse {
	if bl == nil {
		return nil
	}
	resp := &BlocklistResponse{
		DHash: bl.DHash,
		JIDs:  make([]types.JID, 0, len(bl.Items)),
		Items: bl.Items,
	}
	for _, item := range bl.Items {
		if !item.PN.IsEmpty() {
			resp.JIDs = append(resp.JIDs, item.PN)
		} else {
			resp.JIDs = append(resp.JIDs, item.LID)
		}
	}
	return resp
}
