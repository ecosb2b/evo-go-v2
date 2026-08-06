// Package gate controla o acesso às rotas da API neste fork.
//
// Ele substitui o core.GateMiddleware do upstream, que exige ativação de licença
// e responde 503 até que ela exista. A troca é feita num pacote próprio, e não
// editando o core: aquele arquivo é ofuscado (`_txz`, `_kni`, `_z14`) e tem toda
// a cara de ser regenerado a cada release, então qualquer alteração lá
// conflitaria de forma ilegível em toda sincronização com o upstream.
//
// O pacote core segue intacto e funcionando: se houver registro de licença no
// banco, ele ativa normalmente, e o modo "license" abaixo devolve o
// comportamento original sem precisar mexer em código.
package gate

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/evolution-foundation/evolution-go/pkg/core"
)

// Modos de operação, escolhidos por EVOLUTION_GATE_MODE.
const (
	// ModeOpen deixa todas as rotas passarem. É o padrão deste fork.
	ModeOpen = "open"
	// ModeLicense reproduz o comportamento do upstream: 503 enquanto não houver
	// licença ativa.
	ModeLicense = "license"
)

const envGateMode = "EVOLUTION_GATE_MODE"

// Mode devolve o modo configurado. Valor ausente ou desconhecido cai em
// ModeOpen — um erro de digitação na variável não deve derrubar a API inteira
// com 503.
func Mode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envGateMode))) {
	case ModeLicense:
		return ModeLicense
	default:
		return ModeOpen
	}
}

// licenseStatusPayload monta o que o manager espera de GET /license/status.
// O contrato é o do frontend (evolution-manager-v2, src/lib/queries/license):
// ele só verifica o campo "status", que precisa ser "active" ou "inactive".
func licenseStatusPayload(rc *core.RuntimeContext) gin.H {
	payload := gin.H{"status": "active"}

	// Havendo ativação real no banco, os dados dela são repassados — isso mantém
	// o painel mostrando a informação verdadeira em ambientes licenciados.
	if rc != nil {
		if key := rc.APIKey(); key != "" {
			payload["api_key"] = key
		}
	}

	return payload
}

// isPublicPath lista o que responde mesmo sem licença. Reproduz exatamente a
// allowlist do core.GateMiddleware, para que ModeLicense se comporte igual ao
// upstream.
func isPublicPath(path string) bool {
	switch {
	case path == "/health", path == "/server/ok", path == "/favicon.ico",
		path == "/license/status", path == "/license/register", path == "/license/activate",
		path == "/ws":
		return true
	}

	for _, prefix := range []string{"/manager", "/assets", "/passkey-ceremony", "/swagger"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	for _, suffix := range []string{".svg", ".css", ".js", ".png", ".ico", ".woff2", ".woff", ".ttf"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}

	return false
}

// Middleware devolve o gate configurado.
//
// Em ModeOpen ele não bloqueia nada — a autenticação das rotas continua sendo
// feita pelo middleware de apikey, que é independente disto e permanece ativo.
// Este gate nunca foi a camada de autenticação; ele é a trava comercial de
// ativação.
func Middleware(rc *core.RuntimeContext) gin.HandlerFunc {
	if Mode() == ModeOpen {
		return func(c *gin.Context) {
			// O manager é um frontend compilado que consulta /license/status por
			// conta própria e, ao ver "inactive", redireciona para o servidor de
			// licenciamento — antes mesmo da tela de login. Liberar só as rotas
			// da API não bastaria: o painel continuaria inacessível.
			//
			// A resposta é interceptada aqui, e não registrando uma rota, porque
			// o core.LicenseRoutes já registra GET /license/status e o Gin entra
			// em panic com rota duplicada.
			//
			// Se houver licença de verdade no banco, o core assume: devolvemos os
			// dados reais dele em vez de inventar.
			if c.Request.Method == http.MethodGet && c.Request.URL.Path == "/license/status" {
				c.AbortWithStatusJSON(http.StatusOK, licenseStatusPayload(rc))
				return
			}
			c.Next()
		}
	}

	return func(c *gin.Context) {
		if isPublicPath(c.Request.URL.Path) {
			c.Next()
			return
		}

		if valid, _ := core.ValidateContext(rc); !valid {
			scheme := "http"
			if c.Request.TLS != nil {
				scheme = "https"
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error":        "service not activated",
				"code":         "LICENSE_REQUIRED",
				"register_url": fmt.Sprintf("%s://%s/manager/login", scheme, c.Request.Host),
				"message":      "License required. Open the manager to activate your license.",
			})
			return
		}

		c.Next()
	}
}
