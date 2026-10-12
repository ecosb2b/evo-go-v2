package send_service

import (
	"encoding/json"
	"testing"
)

func TestBuildQuotedMessage(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantText     string
		wantImageCap string
	}{
		{
			name:     "id only keeps the legacy empty card",
			body:     `{"messageId":"3EB0A"}`,
			wantText: "",
		},
		{
			name:     "plain text",
			body:     `{"messageId":"3EB0A","text":"Original message"}`,
			wantText: "Original message",
		},
		{
			name:     "message object from the webhook payload",
			body:     `{"messageId":"3EB0A","message":{"conversation":"Original message"}}`,
			wantText: "Original message",
		},
		{
			name:         "non-text message object",
			body:         `{"messageId":"3EB0A","message":{"imageMessage":{"caption":"photo"}}}`,
			wantImageCap: "photo",
		},
		{
			name:     "message object wins over text",
			body:     `{"messageId":"3EB0A","text":"ignored","message":{"conversation":"from message"}}`,
			wantText: "from message",
		},
		{
			name:     "invalid message object falls back to text",
			body:     `{"messageId":"3EB0A","text":"fallback","message":"not an object"}`,
			wantText: "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var q QuotedStruct
			if err := json.Unmarshal([]byte(tt.body), &q); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			msg, _ := buildQuotedMessage(q)
			if tt.wantImageCap != "" {
				if got := msg.GetImageMessage().GetCaption(); got != tt.wantImageCap {
					t.Fatalf("image caption = %q, want %q", got, tt.wantImageCap)
				}
				return
			}
			if msg.Conversation == nil {
				t.Fatalf("conversation is nil, want %q", tt.wantText)
			}
			if got := msg.GetConversation(); got != tt.wantText {
				t.Fatalf("conversation = %q, want %q", got, tt.wantText)
			}
		})
	}
}

// Formato da Evolution API v2: {"key":{"id","participant"},"message":{...}}.
func TestQuotedStructAcceptsV2Key(t *testing.T) {
	var q QuotedStruct
	body := `{"key":{"id":"3EB0A","participant":"5511999999999@s.whatsapp.net"},"message":{"conversation":"hi"}}`
	if err := json.Unmarshal([]byte(body), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	q = q.normalized()
	if q.MessageID != "3EB0A" || q.Participant != "5511999999999@s.whatsapp.net" {
		t.Fatalf("normalized = %+v, want key fields copied", q)
	}

	explicit := QuotedStruct{MessageID: "explicit", Key: &QuotedKey{ID: "from-key"}}.normalized()
	if explicit.MessageID != "explicit" {
		t.Fatalf("messageId = %q, explicit field must win over key", explicit.MessageID)
	}
}
