package utils

import (
	"strconv"
	"strings"
	"time"
)

// StoredQR é o QR guardado na coluna instance.qrcode, no formato
// "<imagem>|<código>|<expira em unix>". Valores gravados antes da validade
// existir têm só dois campos: ExpiresAt fica zero (validade desconhecida).
type StoredQR struct {
	Image     string
	Code      string
	ExpiresAt time.Time
}

// EncodeStoredQR monta o valor da coluna. Nem a imagem (base64) nem o código
// do WhatsApp (campos separados por vírgula) contêm "|".
func EncodeStoredQR(image, code string, expiresAt time.Time) string {
	return image + "|" + code + "|" + strconv.FormatInt(expiresAt.Unix(), 10)
}

// ParseStoredQR lê o valor da coluna; false se estiver vazio ou malformado.
func ParseStoredQR(raw string) (StoredQR, bool) {
	parts := strings.SplitN(raw, "|", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return StoredQR{}, false
	}
	q := StoredQR{Image: parts[0], Code: parts[1]}
	if len(parts) == 3 {
		unix, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return StoredQR{}, false
		}
		q.ExpiresAt = time.Unix(unix, 0)
	}
	return q, true
}

// Expired diz se o código já venceu. Validade desconhecida nunca vence.
func (q StoredQR) Expired(now time.Time) bool {
	return !q.ExpiresAt.IsZero() && !now.Before(q.ExpiresAt)
}
