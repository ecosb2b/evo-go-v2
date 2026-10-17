package receipt_handler

import (
	"net/http"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	receipt_model "github.com/evolution-foundation/evolution-go/pkg/receipt/model"
	receipt_repository "github.com/evolution-foundation/evolution-go/pkg/receipt/repository"
	"github.com/gin-gonic/gin"
)

// MaxIDs é o máximo de mensagens por consulta.
const MaxIDs = 200

type ReceiptHandler interface {
	GetReceipts(ctx *gin.Context)
}

type receiptHandler struct {
	repository receipt_repository.ReceiptRepository
}

func NewReceiptHandler(repository receipt_repository.ReceiptRepository) ReceiptHandler {
	return &receiptHandler{repository: repository}
}

type receiptsRequest struct {
	IDs []string `json:"ids"`
}

// normalizeIDs tira espaços, vazios e repetidos, mantendo a ordem pedida.
func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// GetReceipts godoc
// @Summary Delivery, read and play status of sent messages
// @Description Para cada id: status (sent, delivered, read, played ou unknown) e a data em que cada estado foi visto pela primeira vez. Só mensagens enviadas por esta instância em conversas individuais. Não precisa da instância conectada.
// @Tags Message
// @Accept json
// @Produce json
// @Param data body object{ids=[]string} true "Ids das mensagens (até 200)"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /message/receipts [post]
func (h *receiptHandler) GetReceipts(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data receiptsRequest
	if err := ctx.ShouldBindJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ids := normalizeIDs(data.IDs)
	if len(ids) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}
	if len(ids) > MaxIDs {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "at most 200 ids per request"})
		return
	}

	rows, err := h.repository.Find(instance.Id, ids)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	byID := make(map[string]*receipt_model.MessageReceipt, len(rows))
	for i := range rows {
		byID[rows[i].MessageID] = &rows[i]
	}

	views := make([]receipt_model.View, 0, len(ids))
	for _, id := range ids {
		views = append(views, receipt_model.ToView(id, byID[id]))
	}
	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": views})
}
