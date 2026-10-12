package whatsmeow_service

import (
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

func TestGlobalEventGroup(t *testing.T) {
	tests := map[string]string{
		"Message":              event_types.MESSAGE,
		"SendMessage":          event_types.SEND_MESSAGE,
		"SendStatus":           event_types.SEND_MESSAGE,
		"Receipt":              event_types.READ_RECEIPT,
		"Archive":              event_types.CHAT_PRESENCE,
		"CallOffer":            event_types.CALL,
		"Disconnected":         event_types.CONNECTION,
		"OfflineSyncCompleted": event_types.CONNECTION,
		"StreamReplaced":       event_types.CONNECTION,
		"ClientOutdated":       event_types.CONNECTION,
		"LabelEdit":            event_types.LABEL,
		"PushName":             event_types.CONTACT,
		"Picture":              event_types.PICTURE,
		"UserAbout":            event_types.USER_ABOUT,
		"JoinedGroup":          event_types.GROUP,
		"NewsletterLeave":      event_types.NEWSLETTER,
		"QRCode":               event_types.QRCODE,
		"PairError":            event_types.QRCODE,
		"PasskeyRequest":       event_types.QRCODE,
		"PasskeyConfirmation":  event_types.QRCODE,
		"PasskeyError":         event_types.QRCODE,
		"ButtonClick":          event_types.BUTTON_CLICK,
		"SomethingNew":         "",
	}
	for event, want := range tests {
		if got := globalEventGroup(event); got != want {
			t.Errorf("globalEventGroup(%q) = %q, want %q", event, got, want)
		}
	}
}
