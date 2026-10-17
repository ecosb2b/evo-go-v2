package whatsmeow_service

import (
	receipt_model "github.com/evolution-foundation/evolution-go/pkg/receipt/model"
	"go.mau.fi/whatsmeow/types"
)

// classifyReceipt traduz o tipo do recibo do WhatsApp:
//   - webhookState: o "state" do webhook Receipt;
//   - recordState: o estado gravado no andamento da mensagem ("" = não grava:
//     recibos "Self" são de outro aparelho da própria conta lendo mensagens
//     RECEBIDAS, não das enviadas);
//   - known: false para os recibos internos (retry, sender, server-error,
//     inactive...), que não viram webhook.
//
// Played é o microfone/play azul: o contato ouviu o áudio ou viu o vídeo. O
// WhatsApp o envia mesmo com a confirmação de leitura desligada. Antes era
// descartado.
func classifyReceipt(t types.ReceiptType) (webhookState string, recordState receipt_model.State, known bool) {
	switch t {
	case types.ReceiptTypeDelivered:
		return "Delivered", receipt_model.StateDelivered, true
	case types.ReceiptTypeRead:
		return "Read", receipt_model.StateRead, true
	case types.ReceiptTypeReadSelf:
		return "ReadSelf", "", true
	case types.ReceiptTypePlayed:
		return "Played", receipt_model.StatePlayed, true
	case types.ReceiptTypePlayedSelf:
		return "PlayedSelf", "", true
	default:
		return "", "", false
	}
}
