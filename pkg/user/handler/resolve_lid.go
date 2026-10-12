package user_handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
)

// ResolveLid devolve o número de telefone por trás de um LID
// @Summary Resolve the phone number behind a LID
// @Description Resolves from the local LID store; when the mapping is missing and groupJid is given, the group is refreshed first. 404 when the number is not known yet.
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.ResolveLidStruct true "LID data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 404 {object} gin.H "No mapping known for this LID"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/lid [post]
func (u *userHandler) ResolveLid(ctx *gin.Context) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data user_service.ResolveLidStruct
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if data.Lid == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "lid is required"})
		return
	}

	resp, err := u.userService.ResolveLid(&data, instance)
	switch {
	case err == nil:
		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
	case errors.Is(err, user_service.ErrLIDNotMapped):
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, user_service.ErrInvalidLID), errors.Is(err, user_service.ErrInvalidGroupJID):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		writeUserWAError(ctx, err)
	}
}
