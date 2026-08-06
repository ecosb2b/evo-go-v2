package typebot_handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	typebot_model "github.com/evolution-foundation/evolution-go/pkg/typebot/model"
	typebot_repository "github.com/evolution-foundation/evolution-go/pkg/typebot/repository"
)

type TypebotHandler interface {
	CreateBot(ctx *gin.Context)
	ListBots(ctx *gin.Context)
	UpdateBot(ctx *gin.Context)
	DeleteBot(ctx *gin.Context)
	ListSessions(ctx *gin.Context)
	UpdateSessionStatus(ctx *gin.Context)
	DeleteSession(ctx *gin.Context)
}

type typebotHandler struct {
	typebotRepository typebot_repository.TypebotRepository
	loggerWrapper     *logger_wrapper.LoggerManager
}

func NewTypebotHandler(
	typebotRepository typebot_repository.TypebotRepository,
	loggerWrapper *logger_wrapper.LoggerManager,
) TypebotHandler {
	return &typebotHandler{
		typebotRepository: typebotRepository,
		loggerWrapper:     loggerWrapper,
	}
}

func instanceFromContext(ctx *gin.Context) (*instance_model.Instance, bool) {
	instance, ok := ctx.MustGet("instance").(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return nil, false
	}
	return instance, true
}

// CreateBot godoc
// @Summary Create a Typebot configuration for the instance
// @Tags Typebot
// @Accept json
// @Produce json
// @Param data body typebot_model.TypebotRequest true "Typebot configuration"
// @Success 201 {object} typebot_model.Typebot
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot [post]
func (t *typebotHandler) CreateBot(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	var data typebot_model.TypebotRequest
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.URL == "" || data.Typebot == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "url and typebot are required"})
		return
	}

	bot := &typebot_model.Typebot{
		InstanceID:      instance.Id,
		Enabled:         true,
		Description:     data.Description,
		URL:             data.URL,
		Typebot:         data.Typebot,
		KeywordFinish:   data.KeywordFinish,
		UnknownMessage:  data.UnknownMessage,
		StopBotFromMe:   true,
		ListeningFromMe: false,
	}
	applyRequest(bot, &data)

	if err := t.typebotRepository.CreateBot(bot); err != nil {
		t.loggerWrapper.GetLogger(instance.Id).LogError("[%s] typebot: erro ao criar bot: %v", instance.Id, err)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, bot)
}

// ListBots godoc
// @Summary List the instance's Typebot configurations
// @Tags Typebot
// @Produce json
// @Success 200 {array} typebot_model.Typebot
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot [get]
func (t *typebotHandler) ListBots(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	bots, err := t.typebotRepository.GetBotsByInstanceID(instance.Id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if bots == nil {
		bots = []typebot_model.Typebot{}
	}

	ctx.JSON(http.StatusOK, bots)
}

// UpdateBot godoc
// @Summary Update a Typebot configuration
// @Tags Typebot
// @Accept json
// @Produce json
// @Param id path string true "Typebot id"
// @Param data body typebot_model.TypebotRequest true "Fields to update"
// @Success 200 {object} typebot_model.Typebot
// @Failure 404 {object} gin.H "Not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot/{id} [put]
func (t *typebotHandler) UpdateBot(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	bot, err := t.typebotRepository.GetBotByID(instance.Id, ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if bot == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "typebot not found"})
		return
	}

	var data typebot_model.TypebotRequest
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Só os campos presentes no corpo são gravados, para que um PUT parcial não
	// zere o que não foi enviado.
	if data.Description != "" {
		bot.Description = data.Description
	}
	if data.URL != "" {
		bot.URL = data.URL
	}
	if data.Typebot != "" {
		bot.Typebot = data.Typebot
	}
	if data.KeywordFinish != "" {
		bot.KeywordFinish = data.KeywordFinish
	}
	if data.UnknownMessage != "" {
		bot.UnknownMessage = data.UnknownMessage
	}
	applyRequest(bot, &data)

	if err := t.typebotRepository.UpdateBot(bot); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, bot)
}

// applyRequest grava os campos que aceitam zero como valor legítimo. Eles são
// ponteiros justamente para distinguir "mandou 0" de "não mandou".
func applyRequest(bot *typebot_model.Typebot, data *typebot_model.TypebotRequest) {
	if data.Enabled != nil {
		bot.Enabled = *data.Enabled
	}
	if data.Expire != nil {
		bot.Expire = *data.Expire
	}
	if data.DelayMessage != nil {
		bot.DelayMessage = *data.DelayMessage
	}
	if data.ListeningFromMe != nil {
		bot.ListeningFromMe = *data.ListeningFromMe
	}
	if data.StopBotFromMe != nil {
		bot.StopBotFromMe = *data.StopBotFromMe
	}
}

// DeleteBot godoc
// @Summary Delete a Typebot configuration and its sessions
// @Tags Typebot
// @Produce json
// @Param id path string true "Typebot id"
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot/{id} [delete]
func (t *typebotHandler) DeleteBot(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	if err := t.typebotRepository.DeleteBot(instance.Id, ctx.Param("id")); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// ListSessions godoc
// @Summary List the instance's Typebot sessions
// @Tags Typebot
// @Produce json
// @Success 200 {array} typebot_model.TypebotSession
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot/sessions [get]
func (t *typebotHandler) ListSessions(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	sessions, err := t.typebotRepository.GetSessionsByInstanceID(instance.Id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if sessions == nil {
		sessions = []typebot_model.TypebotSession{}
	}

	ctx.JSON(http.StatusOK, sessions)
}

// UpdateSessionStatus godoc
// @Summary Pause, close or reopen a session
// @Tags Typebot
// @Accept json
// @Produce json
// @Param id path string true "Session id"
// @Param data body object{status=string} true "opened, paused or closed"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot/sessions/{id}/status [put]
func (t *typebotHandler) UpdateSessionStatus(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	var data struct {
		Status string `json:"status"`
	}
	if err := ctx.ShouldBindBodyWithJSON(&data); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch data.Status {
	case typebot_model.SessionOpened, typebot_model.SessionPaused, typebot_model.SessionClosed:
	default:
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "status must be opened, paused or closed"})
		return
	}

	if err := t.typebotRepository.SetSessionStatus(instance.Id, ctx.Param("id"), data.Status); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteSession godoc
// @Summary Delete a Typebot session
// @Tags Typebot
// @Produce json
// @Param id path string true "Session id"
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /typebot/sessions/{id} [delete]
func (t *typebotHandler) DeleteSession(ctx *gin.Context) {
	instance, ok := instanceFromContext(ctx)
	if !ok {
		return
	}

	if err := t.typebotRepository.DeleteSession(instance.Id, ctx.Param("id")); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}
