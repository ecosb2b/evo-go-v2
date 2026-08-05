package send_handler

import (
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

// Send an event message
// @Summary Send a WhatsApp event message
// @Description Cria e envia um Evento/agenda do WhatsApp. `startTime`/`endTime` aceitam
// @Description ISO 8601 (RFC3339) com timezone ou epoch em segundos. Normalmente enviado
// @Description a um JID de grupo, ex.: 1203...@g.us.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.EventStruct true "Event data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/event [post]
func (s *sendHandler) SendEvent(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.EventStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}
	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "event name is required"})
		return
	}
	if data.StartTime.Unix() <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "startTime is required (ISO 8601 or epoch seconds)"})
		return
	}

	message, err := s.sendMessageService.SendEvent(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}
