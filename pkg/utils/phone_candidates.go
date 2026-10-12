package utils

import "strings"

// WhatsAppNumberCandidates devolve as formas do número para consultar no
// IsOnWhatsApp, no formato "+<dígitos>", com a forma preferida primeiro.
//
// Celular brasileiro existe registrado com e sem o 9 depois do DDD: contas
// antigas ficaram com 12 dígitos, contas novas têm 13. O middleware remove o
// 9 (DDD >= 31) antes do serviço, então mandar só a forma recebida errava o
// destinatário registrado com o 9. Consultar as duas numa única chamada deixa
// o WhatsApp dizer qual existe. [upstream#214]
func WhatsAppNumberCandidates(number string) []string {
	digits := onlyDigits(strings.SplitN(number, "@", 2)[0])
	if digits == "" {
		return nil
	}

	withNine, withoutNine, ok := brazilianMobileForms(digits)
	if !ok {
		return []string{"+" + digits}
	}
	return []string{"+" + withNine, "+" + withoutNine}
}

// brazilianMobileForms devolve as formas de 13 e 12 dígitos de um celular
// brasileiro. ok é false para fixo, número de outro país ou formato inválido.
func brazilianMobileForms(digits string) (withNine, withoutNine string, ok bool) {
	if !strings.HasPrefix(digits, "55") {
		return "", "", false
	}
	ddd, subscriber := digits[2:min(4, len(digits))], digits[min(4, len(digits)):]
	if len(ddd) != 2 || ddd[0] == '0' {
		return "", "", false
	}

	switch {
	case len(subscriber) == 9 && subscriber[0] == '9':
		return digits, "55" + ddd + subscriber[1:], true
	case len(subscriber) == 8 && subscriber[0] >= '6':
		// 6 a 9: faixa de celular antes do nono dígito; 2 a 5 é fixo.
		return "55" + ddd + "9" + subscriber, digits, true
	default:
		return "", "", false
	}
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
