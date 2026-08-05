package user_service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

// ============================================================================
// [Athene] Gerenciamento de catálogo de produtos (WhatsApp Business).
//
// Engenharia reversa dos IQs `w:biz:catalog` do Baileys (business.ts), reproduzidos
// no Go via client.DangerousInternals().SendIQ + waBinary.Node.
//
// A imagem do produto NÃO usa o pipeline padrão do whatsmeow: o Baileys sobe em
// `/product/image` SEM criptografia (HKDF vazio). Por isso o upload é feito à mão
// aqui, usando RefreshMediaConn (auth + host) e um POST direto.
// ============================================================================

// ProductCreateStruct é o body de POST /catalog/product.
type ProductCreateStruct struct {
	Name        string `json:"name" example:"Camiseta Athene"`
	Description string `json:"description,omitempty"`
	// Preço em miliunidades da moeda: R$ 10,00 => 10000. (padrão do WhatsApp)
	Price       int64  `json:"price" example:"10000"`
	Currency    string `json:"currency" example:"BRL"`
	RetailerId  string `json:"retailerId,omitempty"`
	// Forneça UMA das opções de imagem:
	ImageBase64 string `json:"imageBase64,omitempty"` // base64 do arquivo (com ou sem prefixo data:)
	ImageURL    string `json:"imageUrl,omitempty"`    // URL externa (será baixada) ou uma URL .whatsapp.net (reusada)
	IsHidden    bool   `json:"isHidden,omitempty"`
	// País de origem (ISO 3166-1 alpha-2, ex.: "BR"). O WhatsApp passou a exigir
	// esse campo no cadastro de produto. Se vazio, envia COUNTRY_ORIGIN_EXEMPT
	// (comportamento antigo do Baileys), que tende a ser rejeitado.
	OriginCountryCode string `json:"originCountryCode,omitempty" example:"BR"`
}

// ProductUpdateStruct é o body de PUT /catalog/product. Campos vazios não são
// alterados (atualização parcial). Envie imagem só se quiser trocá-la.
type ProductUpdateStruct struct {
	ProductId   string `json:"productId" example:"1234567890"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Price       int64  `json:"price,omitempty"`
	Currency    string `json:"currency,omitempty"`
	RetailerId  string `json:"retailerId,omitempty"`
	ImageBase64 string `json:"imageBase64,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	IsHidden    *bool  `json:"isHidden,omitempty"`
	// País de origem (ISO 3166-1 alpha-2, ex.: "BR").
	OriginCountryCode string `json:"originCountryCode,omitempty" example:"BR"`
}

// ProductDeleteStruct é o body de DELETE /catalog/product.
type ProductDeleteStruct struct {
	ProductIds []string `json:"productIds"`
}

// ---------- helpers de node ----------

func catChildByTag(n *waBinary.Node, tag string) *waBinary.Node {
	if n == nil {
		return nil
	}
	for _, c := range n.GetChildren() {
		if c.Tag == tag {
			cc := c
			return &cc
		}
	}
	return nil
}

func catNodeText(n *waBinary.Node) string {
	if n == nil {
		return ""
	}
	switch v := n.Content.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	}
	return ""
}

func catTextNode(tag, value string) waBinary.Node {
	return waBinary.Node{Tag: tag, Content: []byte(value)}
}

// uploadCatalogImage sobe a imagem (sem criptografia) para /product/image e
// devolve a URL hospedada na WhatsApp, para referenciar no node do produto.
func (u *userService) uploadCatalogImage(client *whatsmeow.Client, img []byte) (string, error) {
	mc, err := client.DangerousInternals().RefreshMediaConn(context.Background(), false)
	if err != nil {
		return "", fmt.Errorf("failed to get media conn: %w", err)
	}
	if mc == nil || len(mc.Hosts) == 0 {
		return "", errors.New("no media hosts available")
	}

	sum := sha256.Sum256(img)
	token := base64.URLEncoding.EncodeToString(sum[:])

	q := url.Values{}
	q.Set("auth", mc.Auth)
	q.Set("token", token)
	uploadURL := fmt.Sprintf("https://%s/product/image/%s?%s", mc.Hosts[0].Hostname, token, q.Encode())

	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(img))
	if err != nil {
		return "", err
	}
	req.ContentLength = int64(len(img))
	req.Header.Set("Origin", "https://web.whatsapp.com")
	req.Header.Set("Referer", "https://web.whatsapp.com/")
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("catalog image upload request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("catalog image upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var ur struct {
		URL        string `json:"url"`
		DirectPath string `json:"direct_path"`
	}
	if err := json.Unmarshal(body, &ur); err != nil {
		return "", fmt.Errorf("failed to parse upload response: %w (body: %s)", err, string(body))
	}
	if ur.URL != "" {
		return ur.URL, nil
	}
	if ur.DirectPath != "" {
		return "https://mmg.whatsapp.net" + ur.DirectPath, nil
	}
	return "", fmt.Errorf("upload response missing url/direct_path: %s", string(body))
}

func catFetchImageBytes(imageURL string) ([]byte, error) {
	resp, err := http.Get(imageURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch image, status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (u *userService) resolveProductImageURL(client *whatsmeow.Client, data *ProductCreateStruct) (string, error) {
	// URL já hospedada na WhatsApp: reusa sem re-upload (igual Baileys).
	if strings.Contains(data.ImageURL, "whatsapp.net") {
		return data.ImageURL, nil
	}
	var img []byte
	var err error
	switch {
	case data.ImageBase64 != "":
		b64 := data.ImageBase64
		if strings.HasPrefix(b64, "data:") {
			if i := strings.Index(b64, ","); i > 0 {
				b64 = b64[i+1:]
			}
		}
		img, err = base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return "", fmt.Errorf("invalid imageBase64: %w", err)
		}
	case data.ImageURL != "":
		img, err = catFetchImageBytes(data.ImageURL)
		if err != nil {
			return "", fmt.Errorf("failed to download imageUrl: %w", err)
		}
	default:
		return "", errors.New("image required: provide imageBase64 or imageUrl")
	}
	return u.uploadCatalogImage(client, img)
}

// catParseProductNode extrai os campos de um <product> da resposta.
func catParseProductNode(product *waBinary.Node) map[string]any {
	if product == nil {
		return nil
	}
	media := catChildByTag(product, "media")
	image := catChildByTag(media, "image")
	statusInfo := catChildByTag(product, "status_info")

	imageURL := ""
	if image != nil {
		if o := catChildByTag(image, "original_image_url"); o != nil {
			imageURL = catNodeText(o)
		} else if r := catChildByTag(image, "request_image_url"); r != nil {
			imageURL = catNodeText(r)
		} else if uu := catChildByTag(image, "url"); uu != nil {
			imageURL = catNodeText(uu)
		}
	}

	out := map[string]any{
		"id":          catNodeText(catChildByTag(product, "id")),
		"name":        catNodeText(catChildByTag(product, "name")),
		"description": catNodeText(catChildByTag(product, "description")),
		"price":       catNodeText(catChildByTag(product, "price")),
		"currency":    catNodeText(catChildByTag(product, "currency")),
		"retailerId":  catNodeText(catChildByTag(product, "retailer_id")),
		"url":         catNodeText(catChildByTag(product, "url")),
		"isHidden":    product.AttrGetter().OptionalString("is_hidden") == "true",
		"imageUrl":    imageURL,
	}
	if statusInfo != nil {
		out["reviewStatus"] = catNodeText(catChildByTag(statusInfo, "status"))
	}
	return out
}

// CreateProduct cria um produto no catálogo da instância (IQ product_catalog_add).
func (u *userService) CreateProduct(data *ProductCreateStruct, instance *instance_model.Instance) (map[string]any, error) {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	if data.Name == "" {
		return nil, errors.New("name is required")
	}
	if data.Currency == "" {
		return nil, errors.New("currency is required")
	}

	imageURL, err := u.resolveProductImageURL(client, data)
	if err != nil {
		return nil, err
	}

	content := []waBinary.Node{catTextNode("name", data.Name)}
	if data.Description != "" {
		content = append(content, catTextNode("description", data.Description))
	}
	if data.RetailerId != "" {
		content = append(content, catTextNode("retailer_id", data.RetailerId))
	}
	content = append(content, waBinary.Node{
		Tag: "media",
		Content: []waBinary.Node{{
			Tag:     "image",
			Content: []waBinary.Node{catTextNode("url", imageURL)},
		}},
	})
	content = append(content, catTextNode("price", strconv.FormatInt(data.Price, 10)))
	content = append(content, catTextNode("currency", data.Currency))

	attrs := waBinary.Attrs{
		"is_hidden": strconv.FormatBool(data.IsHidden),
	}
	// País de origem: o WhatsApp exige compliance_info. Sem país, cai no antigo
	// COUNTRY_ORIGIN_EXEMPT do Baileys (tende a ser rejeitado hoje).
	if data.OriginCountryCode != "" {
		content = append(content, waBinary.Node{
			Tag:     "compliance_info",
			Content: []waBinary.Node{catTextNode("country_code_origin", data.OriginCountryCode)},
		})
	} else {
		attrs["compliance_category"] = "COUNTRY_ORIGIN_EXEMPT"
	}

	addNode := waBinary.Node{
		Tag:   "product_catalog_add",
		Attrs: waBinary.Attrs{"v": "1"},
		Content: []waBinary.Node{
			{Tag: "product", Attrs: attrs, Content: content},
			catTextNode("width", "100"),
			catTextNode("height", "100"),
		},
	}

	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] CATALOG-DEBUG add stanza: %s", instance.Id, &addNode)

	resp, err := client.DangerousInternals().SendIQ(context.Background(), whatsmeow.DangerousInfoQuery{
		Namespace: "w:biz:catalog",
		Type:      whatsmeow.DangerousInfoQueryType("set"),
		To:        types.ServerJID,
		Content:   []waBinary.Node{addNode},
	})
	if err != nil {
		u.loggerWrapper.GetLogger(instance.Id).LogError("[%s] product_catalog_add failed: %v", instance.Id, err)
		return nil, err
	}
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] CATALOG-DEBUG add response: %s", instance.Id, resp)

	addResp := catChildByTag(resp, "product_catalog_add")
	product := catParseProductNode(catChildByTag(addResp, "product"))
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Product created in catalog: %s", instance.Id, data.Name)
	if product == nil {
		return map[string]any{"message": "success"}, nil
	}
	return product, nil
}

// UpdateProduct edita um produto existente no catálogo (IQ product_catalog_edit).
// Atualização parcial: só os campos preenchidos são enviados.
func (u *userService) UpdateProduct(data *ProductUpdateStruct, instance *instance_model.Instance) (map[string]any, error) {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	if data.ProductId == "" {
		return nil, errors.New("productId is required")
	}

	content := []waBinary.Node{catTextNode("id", data.ProductId)}
	if data.Name != "" {
		content = append(content, catTextNode("name", data.Name))
	}
	if data.Description != "" {
		content = append(content, catTextNode("description", data.Description))
	}
	if data.RetailerId != "" {
		content = append(content, catTextNode("retailer_id", data.RetailerId))
	}
	if data.ImageBase64 != "" || data.ImageURL != "" {
		imageURL, err := u.resolveProductImageURL(client, &ProductCreateStruct{
			ImageBase64: data.ImageBase64,
			ImageURL:    data.ImageURL,
		})
		if err != nil {
			return nil, err
		}
		content = append(content, waBinary.Node{
			Tag: "media",
			Content: []waBinary.Node{{
				Tag:     "image",
				Content: []waBinary.Node{catTextNode("url", imageURL)},
			}},
		})
	}
	if data.Price > 0 {
		content = append(content, catTextNode("price", strconv.FormatInt(data.Price, 10)))
	}
	if data.Currency != "" {
		content = append(content, catTextNode("currency", data.Currency))
	}

	attrs := waBinary.Attrs{}
	if data.IsHidden != nil {
		attrs["is_hidden"] = strconv.FormatBool(*data.IsHidden)
	}
	if data.OriginCountryCode != "" {
		content = append(content, waBinary.Node{
			Tag:     "compliance_info",
			Content: []waBinary.Node{catTextNode("country_code_origin", data.OriginCountryCode)},
		})
	}

	editNode := waBinary.Node{
		Tag:   "product_catalog_edit",
		Attrs: waBinary.Attrs{"v": "1"},
		Content: []waBinary.Node{
			{Tag: "product", Attrs: attrs, Content: content},
			catTextNode("width", "100"),
			catTextNode("height", "100"),
		},
	}

	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] CATALOG-DEBUG edit stanza: %s", instance.Id, &editNode)

	resp, err := client.DangerousInternals().SendIQ(context.Background(), whatsmeow.DangerousInfoQuery{
		Namespace: "w:biz:catalog",
		Type:      whatsmeow.DangerousInfoQueryType("set"),
		To:        types.ServerJID,
		Content:   []waBinary.Node{editNode},
	})
	if err != nil {
		u.loggerWrapper.GetLogger(instance.Id).LogError("[%s] product_catalog_edit failed: %v", instance.Id, err)
		return nil, err
	}
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] CATALOG-DEBUG edit response: %s", instance.Id, resp)

	editResp := catChildByTag(resp, "product_catalog_edit")
	product := catParseProductNode(catChildByTag(editResp, "product"))
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Product updated in catalog: %s", instance.Id, data.ProductId)
	if product == nil {
		return map[string]any{"message": "success"}, nil
	}
	return product, nil
}

// GetCatalog lista os produtos do catálogo da instância (IQ product_catalog get).
func (u *userService) GetCatalog(instance *instance_model.Instance) ([]map[string]any, error) {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}
	if client.Store == nil || client.Store.ID == nil {
		return nil, errors.New("instance not logged in")
	}
	ownJID := client.Store.ID.ToNonAD()

	catalogNode := waBinary.Node{
		Tag: "product_catalog",
		Attrs: waBinary.Attrs{
			"jid":               ownJID,
			"allow_shop_source": "true",
		},
		Content: []waBinary.Node{
			catTextNode("limit", "100"),
			catTextNode("width", "100"),
			catTextNode("height", "100"),
		},
	}

	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] CATALOG-DEBUG list stanza: %s (ownJID=%s)", instance.Id, &catalogNode, ownJID.String())

	resp, err := client.DangerousInternals().SendIQ(context.Background(), whatsmeow.DangerousInfoQuery{
		Namespace: "w:biz:catalog",
		Type:      whatsmeow.DangerousInfoQueryType("get"),
		To:        types.ServerJID,
		Content:   []waBinary.Node{catalogNode},
	})
	if err != nil {
		u.loggerWrapper.GetLogger(instance.Id).LogError("[%s] product_catalog list failed: %v", instance.Id, err)
		return nil, err
	}
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] CATALOG-DEBUG list response: %s", instance.Id, resp)

	products := []map[string]any{}
	if pc := catChildByTag(resp, "product_catalog"); pc != nil {
		for _, c := range pc.GetChildren() {
			if c.Tag == "product" {
				cc := c
				if p := catParseProductNode(&cc); p != nil {
					products = append(products, p)
				}
			}
		}
	}
	return products, nil
}

// DeleteProducts remove produtos do catálogo (IQ product_catalog_delete).
func (u *userService) DeleteProducts(data *ProductDeleteStruct, instance *instance_model.Instance) (int, error) {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return 0, err
	}
	if len(data.ProductIds) == 0 {
		return 0, errors.New("productIds is required")
	}

	prods := make([]waBinary.Node, 0, len(data.ProductIds))
	for _, id := range data.ProductIds {
		prods = append(prods, waBinary.Node{
			Tag:     "product",
			Content: []waBinary.Node{catTextNode("id", id)},
		})
	}

	delNode := waBinary.Node{
		Tag:     "product_catalog_delete",
		Attrs:   waBinary.Attrs{"v": "1"},
		Content: prods,
	}

	resp, err := client.DangerousInternals().SendIQ(context.Background(), whatsmeow.DangerousInfoQuery{
		Namespace: "w:biz:catalog",
		Type:      whatsmeow.DangerousInfoQueryType("set"),
		To:        types.ServerJID,
		Content:   []waBinary.Node{delNode},
	})
	if err != nil {
		return 0, err
	}

	deleted := 0
	if dn := catChildByTag(resp, "product_catalog_delete"); dn != nil {
		if v := dn.AttrGetter().OptionalString("deleted_count"); v != "" {
			deleted, _ = strconv.Atoi(v)
		}
	}
	u.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Deleted %d product(s) from catalog", instance.Id, deleted)
	return deleted, nil
}
