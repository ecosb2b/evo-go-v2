package message_handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	message_service "github.com/evolution-foundation/evolution-go/pkg/message/service"
)

// SubscribePresence assina a presença de um contato
// @Summary Subscribe to a contact's presence
// @Description Subscribe to a contact's presence (online / last seen) so the instance receives Presence events for that number until the next reconnect. WhatsApp only delivers them while the instance is online (alwaysOnline=true).
// @Tags Message
// @Accept json
// @Produce json
// @Param message body message_service.SubscribePresenceStruct true "Number to subscribe to"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /message/subscribe [post]
func (m *messageHandler) SubscribePresence(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data message_service.SubscribePresenceStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "number is required"})
		return
	}

	result, err := m.messageService.SubscribePresence(&data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": result})
}
