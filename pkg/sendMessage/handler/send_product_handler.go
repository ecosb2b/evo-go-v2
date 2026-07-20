package send_handler

import (
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

// Send a catalog product card
// @Summary Send a catalog product card
// @Description Envia o card de um produto do catálogo (foto, título, preço e botão)
// @Description para um contato. Os produtos são criados no app oficial / Meta Commerce
// @Description Manager; aqui você informa o `productId` e os dados exibidos no card.
// @Description `price` é em miliunidades da moeda (R$ 10,00 = 10000).
// @Tags Send
// @Accept json
// @Produce json
// @Param message body send_service.ProductStruct true "Product message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/product [post]
func (s *sendHandler) SendProduct(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ProductStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}
	if data.ProductId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "productId is required"})
		return
	}
	if data.Title == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	if data.Currency == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "currency is required"})
		return
	}
	if data.ImageBase64 == "" && data.ImageURL == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "imageBase64 or imageUrl is required"})
		return
	}

	message, err := s.sendMessageService.SendProduct(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}
