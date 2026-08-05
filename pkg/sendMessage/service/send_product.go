package send_service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
// [Athene] Envio de card de produto do catálogo (waE2E.ProductMessage).
//
// Diferente do CRUD de catálogo (IQ `w:biz:catalog`, que a Meta desativou), isto
// é uma MENSAGEM comum — mesmo caminho de envio de texto/mídia — então funciona
// com o protocolo atual. Os produtos são criados/gerenciados no app oficial ou no
// Meta Commerce Manager; aqui a API só dispara o card na conversa.
// ============================================================================

// ProductStruct é o body de POST /send/product.
type ProductStruct struct {
	Number string `json:"number"`
	Id     string `json:"id,omitempty"`

	// Dados do produto (aparecem no card). O ProductId deve ser o ID do produto
	// no seu catálogo — pegue no app oficial / Commerce Manager.
	ProductId   string `json:"productId"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Preço em miliunidades da moeda: R$ 10,00 => 10000.
	Price       int64  `json:"price"`
	Currency    string `json:"currency" example:"BRL"`
	RetailerId  string `json:"retailerId,omitempty"`
	Url         string `json:"url,omitempty"`

	// Imagem do produto: base64 ou URL externa (será baixada e reenviada).
	ImageBase64 string `json:"imageBase64,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`

	// JID do dono do catálogo. Se vazio, usa o JID da própria instância.
	BusinessOwnerJid string `json:"businessOwnerJid,omitempty"`

	Body   string `json:"body,omitempty"`
	Footer string `json:"footer,omitempty"`

	Delay        int32        `json:"delay"`
	MentionedJID []string     `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	FormatJid    *bool        `json:"formatJid,omitempty"`
	Quoted       QuotedStruct `json:"quoted"`
}

func productImageBytes(data *ProductStruct) ([]byte, error) {
	if data.ImageBase64 != "" {
		b64 := data.ImageBase64
		if strings.HasPrefix(b64, "data:") {
			if i := strings.Index(b64, ","); i > 0 {
				b64 = b64[i+1:]
			}
		}
		img, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("invalid imageBase64: %w", err)
		}
		return img, nil
	}
	if data.ImageURL != "" {
		resp, err := http.Get(data.ImageURL)
		if err != nil {
			return nil, fmt.Errorf("failed to download imageUrl: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to download imageUrl, status %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return nil, errors.New("image required: provide imageBase64 or imageUrl")
}

// SendProduct envia o card de um produto do catálogo para um contato.
func (s *sendService) SendProduct(data *ProductStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	if data.ProductId == "" {
		return nil, errors.New("productId is required")
	}
	if data.Title == "" {
		return nil, errors.New("title is required")
	}
	if data.Currency == "" {
		return nil, errors.New("currency is required")
	}

	// Imagem do produto: upload padrão de imagem (suportado nativamente pelo whatsmeow).
	imgBytes, err := productImageBytes(data)
	if err != nil {
		return nil, err
	}
	uploaded, err := client.Upload(context.Background(), imgBytes, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("failed to upload product image: %w", err)
	}
	productImage := &waE2E.ImageMessage{
		Mimetype:      proto.String("image/jpeg"),
		URL:           &uploaded.URL,
		DirectPath:    &uploaded.DirectPath,
		MediaKey:      uploaded.MediaKey,
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    &uploaded.FileLength,
	}

	// Dono do catálogo: por padrão a própria instância.
	ownerJid := data.BusinessOwnerJid
	if ownerJid == "" {
		if client.Store == nil || client.Store.ID == nil {
			return nil, errors.New("instance not logged in")
		}
		ownerJid = client.Store.ID.ToNonAD().String()
	}

	snapshot := &waE2E.ProductMessage_ProductSnapshot{
		ProductImage:      productImage,
		ProductID:         proto.String(data.ProductId),
		Title:             proto.String(data.Title),
		CurrencyCode:      proto.String(data.Currency),
		PriceAmount1000:   proto.Int64(data.Price),
		ProductImageCount: proto.Uint32(1),
	}
	if data.Description != "" {
		snapshot.Description = proto.String(data.Description)
	}
	if data.RetailerId != "" {
		snapshot.RetailerID = proto.String(data.RetailerId)
	}
	if data.Url != "" {
		snapshot.URL = proto.String(data.Url)
	}

	productMsg := &waE2E.ProductMessage{
		Product:          snapshot,
		BusinessOwnerJID: proto.String(ownerJid),
	}
	if data.Body != "" {
		productMsg.Body = proto.String(data.Body)
	}
	if data.Footer != "" {
		productMsg.Footer = proto.String(data.Footer)
	}

	msg := &waE2E.Message{ProductMessage: productMsg}

	message, err := s.SendMessage(instance, msg, "ProductMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
	if err != nil {
		return nil, err
	}

	return message, nil
}
