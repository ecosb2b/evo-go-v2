package send_service

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestApplyViewOnce(t *testing.T) {
	image := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{}}
	applyViewOnce(image)
	if !image.GetImageMessage().GetViewOnce() {
		t.Fatal("image not marked view-once")
	}

	video := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}
	applyViewOnce(video)
	if !video.GetVideoMessage().GetViewOnce() {
		t.Fatal("video not marked view-once")
	}

	audio := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}
	applyViewOnce(audio)
	if !audio.GetAudioMessage().GetViewOnce() {
		t.Fatal("audio not marked view-once")
	}

	doc := &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{}}
	applyViewOnce(doc) // documento não tem o campo; não pode quebrar
	applyViewOnce(nil)
}
