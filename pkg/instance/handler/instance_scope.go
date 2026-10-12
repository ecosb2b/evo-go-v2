package instance_handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// authorizeInstanceScope garante que a requisição só atue sobre a instância a
// que a credencial dá acesso. A Global API Key ("admin" no contexto) alcança
// qualquer instância; o token de uma instância só alcança ela mesma.
//
// Sem isso, rotas com :instanceId no path aceitavam o token de qualquer
// instância e liam ou alteravam a configuração de outra. [upstream#136]
func authorizeInstanceScope(ctx *gin.Context, instanceID string) bool {
	if admin, _ := ctx.Get("admin"); admin == true {
		return true
	}

	value, exists := ctx.Get("instance")
	instance, ok := value.(*instance_model.Instance)
	if !exists || !ok || instance == nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return false
	}

	if instance.Id != instanceID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "token does not grant access to this instance"})
		return false
	}

	return true
}
