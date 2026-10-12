package whatsmeow_service

import "github.com/evolution-foundation/evolution-go/pkg/internal/event_types"

// globalEventGroup devolve o grupo de assinatura (MESSAGE, CONNECTION...) de
// um evento emitido, ou "" se ele não pertence a nenhum. As filas globais do
// AMQP e do NATS tinham cada uma a sua cópia deste mapeamento, e elas
// divergiram: ButtonClick, Picture, UserAbout e os eventos de passkey e de
// offline sync não chegavam a uma ou às duas. (#193, #105)
func globalEventGroup(eventType string) string {
	switch eventType {
	case "Message":
		return event_types.MESSAGE
	case "SendMessage", "SendStatus":
		return event_types.SEND_MESSAGE
	case "Receipt":
		return event_types.READ_RECEIPT
	case "Presence":
		return event_types.PRESENCE
	case "HistorySync":
		return event_types.HISTORY_SYNC
	case "ChatPresence", "Archive":
		return event_types.CHAT_PRESENCE
	case "CallOffer", "CallAccept", "CallTerminate", "CallOfferNotice", "CallRelayLatency":
		return event_types.CALL
	case "Connected", "PairSuccess", "TemporaryBan", "LoggedOut", "ConnectFailure",
		"Disconnected", "KeepAliveTimeout", "KeepAliveRestored", "OfflineSyncCompleted",
		"StreamReplaced", "ClientOutdated":
		return event_types.CONNECTION
	case "LabelEdit", "LabelAssociationChat", "LabelAssociationMessage":
		return event_types.LABEL
	case "Contact", "PushName":
		return event_types.CONTACT
	case "Picture":
		return event_types.PICTURE
	case "UserAbout":
		return event_types.USER_ABOUT
	case "GroupInfo", "JoinedGroup":
		return event_types.GROUP
	case "NewsletterJoin", "NewsletterLeave":
		return event_types.NEWSLETTER
	case "QRCode", "QRTimeout", "QRSuccess", "PairError", "PasskeyRequest", "PasskeyConfirmation", "PasskeyError":
		return event_types.QRCODE
	case "ButtonClick":
		return event_types.BUTTON_CLICK
	default:
		return ""
	}
}
