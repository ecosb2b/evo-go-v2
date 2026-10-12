package send_service

import (
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// applyViewOnce marca a mídia como visualização única. Documento e figurinha
// não suportam e seguem como mensagem normal. [upstream#147]
func applyViewOnce(media *waE2E.Message) {
	if media == nil {
		return
	}
	if media.ImageMessage != nil {
		media.ImageMessage.ViewOnce = proto.Bool(true)
	}
	if media.VideoMessage != nil {
		media.VideoMessage.ViewOnce = proto.Bool(true)
	}
	if media.AudioMessage != nil {
		media.AudioMessage.ViewOnce = proto.Bool(true)
	}
	if media.PtvMessage != nil {
		media.PtvMessage.ViewOnce = proto.Bool(true)
	}
}
