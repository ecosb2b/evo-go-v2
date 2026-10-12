package send_service

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func TestParticipantMentionJID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    types.GroupParticipant
		want string
	}{
		{
			name: "prefers_phone_number_over_lid",
			p: types.GroupParticipant{
				JID:         types.NewJID("123456789012345", types.HiddenUserServer),
				PhoneNumber: types.NewJID("5511999999999", types.DefaultUserServer),
			},
			want: "5511999999999@s.whatsapp.net",
		},
		{
			name: "falls_back_to_jid_when_phone_empty",
			p: types.GroupParticipant{
				JID: types.NewJID("5511888888888", types.DefaultUserServer),
			},
			want: "5511888888888@s.whatsapp.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := participantMentionJID(tt.p)
			if got != tt.want {
				t.Fatalf("participantMentionJID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetMessageMentionedJIDs(t *testing.T) {
	t.Parallel()

	mentioned := []string{"5511999999999@s.whatsapp.net", "5511888888888@s.whatsapp.net"}

	tests := []struct {
		name        string
		messageType string
		msg         *waE2E.Message
		wantFrom    func(msg *waE2E.Message) []string
	}{
		{
			name:        "doc_without_caption",
			messageType: "DocumentMessage",
			msg: &waE2E.Message{
				DocumentMessage: &waE2E.DocumentMessage{
					Caption: proto.String(""),
				},
			},
			wantFrom: func(msg *waE2E.Message) []string {
				return msg.DocumentMessage.ContextInfo.MentionedJID
			},
		},
		{
			name:        "doc_with_caption",
			messageType: "DocumentMessage",
			msg: &waE2E.Message{
				DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
					Message: &waE2E.Message{
						DocumentMessage: &waE2E.DocumentMessage{
							Caption: proto.String("hello @all"),
						},
					},
				},
			},
			wantFrom: func(msg *waE2E.Message) []string {
				return msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo.MentionedJID
			},
		},
		{
			name:        "image",
			messageType: "ImageMessage",
			msg: &waE2E.Message{
				ImageMessage: &waE2E.ImageMessage{
					Caption: proto.String("caption"),
				},
			},
			wantFrom: func(msg *waE2E.Message) []string {
				return msg.ImageMessage.ContextInfo.MentionedJID
			},
		},
		{
			name:        "video",
			messageType: "VideoMessage",
			msg: &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					Caption: proto.String("caption"),
				},
			},
			wantFrom: func(msg *waE2E.Message) []string {
				return msg.VideoMessage.ContextInfo.MentionedJID
			},
		},
		{
			name:        "extended_text",
			messageType: "ExtendedTextMessage",
			msg: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{
					Text: proto.String("@todos"),
				},
			},
			wantFrom: func(msg *waE2E.Message) []string {
				return msg.ExtendedTextMessage.ContextInfo.MentionedJID
			},
		},
		{
			name:        "event",
			messageType: "EventMessage",
			msg: &waE2E.Message{
				EventMessage: &waE2E.EventMessage{},
			},
			wantFrom: func(msg *waE2E.Message) []string {
				return msg.EventMessage.ContextInfo.MentionedJID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			setMessageMentionedJIDs(tt.msg, tt.messageType, mentioned)
			got := tt.wantFrom(tt.msg)
			if len(got) != len(mentioned) {
				t.Fatalf("MentionedJID len = %d, want %d (got %#v)", len(got), len(mentioned), got)
			}
			for i := range mentioned {
				if got[i] != mentioned[i] {
					t.Fatalf("MentionedJID[%d] = %q, want %q", i, got[i], mentioned[i])
				}
			}
		})
	}
}

func TestSetMessageMentionedJIDs_DocWithCaptionDoesNotPanic(t *testing.T) {
	t.Parallel()

	// Reproduces the #114 failure mode: DocumentMessage is nil after wrap.
	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				DocumentMessage: &waE2E.DocumentMessage{
					Caption: proto.String("doc caption"),
				},
			},
		},
	}

	setMessageMentionedJIDs(msg, "DocumentMessage", []string{"5511999999999@s.whatsapp.net"})

	got := msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo.MentionedJID
	if len(got) != 1 || got[0] != "5511999999999@s.whatsapp.net" {
		t.Fatalf("unexpected MentionedJID: %#v", got)
	}
	if msg.DocumentMessage != nil {
		t.Fatal("DocumentMessage should remain nil for captioned documents")
	}
}

// Botões, listas, carrossel e produto perdiam as menções sem aviso.
func TestSetMessageMentionedJIDsInteractiveTypes(t *testing.T) {
	jids := []string{"5511999999999@s.whatsapp.net"}

	tests := []struct {
		name        string
		messageType string
		msg         *waE2E.Message
		get         func(*waE2E.Message) []string
	}{
		{
			name:        "interactive",
			messageType: "InteractiveMessage",
			msg:         &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{}},
			get:         func(m *waE2E.Message) []string { return m.GetInteractiveMessage().GetContextInfo().GetMentionedJID() },
		},
		{
			name:        "list",
			messageType: "ListMessage",
			msg:         &waE2E.Message{ListMessage: &waE2E.ListMessage{}},
			get:         func(m *waE2E.Message) []string { return m.GetListMessage().GetContextInfo().GetMentionedJID() },
		},
		{
			name:        "buttons",
			messageType: "ButtonsMessage",
			msg:         &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}},
			get:         func(m *waE2E.Message) []string { return m.GetButtonsMessage().GetContextInfo().GetMentionedJID() },
		},
		{
			name:        "buttons wrapped in document with caption",
			messageType: "ButtonsMessage",
			msg: &waE2E.Message{DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{ButtonsMessage: &waE2E.ButtonsMessage{}},
			}},
			get: func(m *waE2E.Message) []string {
				return m.GetDocumentWithCaptionMessage().GetMessage().GetButtonsMessage().GetContextInfo().GetMentionedJID()
			},
		},
		{
			name:        "product",
			messageType: "ProductMessage",
			msg:         &waE2E.Message{ProductMessage: &waE2E.ProductMessage{}},
			get:         func(m *waE2E.Message) []string { return m.GetProductMessage().GetContextInfo().GetMentionedJID() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMessageMentionedJIDs(tt.msg, tt.messageType, jids)
			if got := tt.get(tt.msg); len(got) != 1 || got[0] != jids[0] {
				t.Fatalf("mentions = %v, want %v", got, jids)
			}
		})
	}
}

func TestSetMessageMentionedJIDsInteractiveInViewOnce(t *testing.T) {
	msg := &waE2E.Message{ViewOnceMessage: &waE2E.FutureProofMessage{
		Message: &waE2E.Message{InteractiveMessage: &waE2E.InteractiveMessage{}},
	}}
	setMessageMentionedJIDs(msg, "InteractiveMessage", []string{"5511999999999@s.whatsapp.net"})
	if got := msg.GetViewOnceMessage().GetMessage().GetInteractiveMessage().GetContextInfo().GetMentionedJID(); len(got) != 1 {
		t.Fatalf("mentions = %v, want one", got)
	}
}
