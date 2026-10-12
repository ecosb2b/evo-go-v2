package user_service

import (
	"context"
	"errors"
	"strings"

	"go.mau.fi/whatsmeow/types"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
)

// ErrLIDNotMapped: o número por trás do LID ainda não é conhecido localmente.
var ErrLIDNotMapped = errors.New("no phone number mapping found for this lid")

// Erros de entrada, para o handler responder 400.
var (
	ErrInvalidLID      = errors.New("invalid lid")
	ErrInvalidGroupJID = errors.New("invalid groupJid")
)

type ResolveLidStruct struct {
	Lid string `json:"lid"`
	// GroupJid opcional: grupo onde o LID aparece. Se o mapeamento não estiver
	// no store, reler o grupo costuma trazê-lo.
	GroupJid string `json:"groupJid,omitempty"`
}

type ResolveLidResult struct {
	Lid         string `json:"lid"`
	PhoneNumber string `json:"phoneNumber"`
	JID         string `json:"jid"`
}

// parseLID aceita "123@lid" ou só os dígitos.
func parseLID(raw string) (types.JID, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "@") {
		raw += "@" + types.HiddenUserServer
	}
	jid, err := types.ParseJID(raw)
	if err != nil || jid.Server != types.HiddenUserServer || jid.User == "" {
		return types.JID{}, ErrInvalidLID
	}
	return jid.ToNonAD(), nil
}

// ResolveLid devolve o número associado a um LID. Só lê o store local do
// whatsmeow: o protocolo não tem consulta LID→número (apenas o inverso), e o
// mapeamento chega passivamente (mensagem do contato, grupo com participantes
// LID). Com groupJid, relê o grupo como tentativa. [upstream#179]
func (u *userService) ResolveLid(data *ResolveLidStruct, instance *instance_model.Instance) (*ResolveLidResult, error) {
	client, err := u.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	lid, err := parseLID(data.Lid)
	if err != nil {
		return nil, err
	}
	if client.Store.LIDs == nil {
		return nil, errors.New("lid store unavailable")
	}

	ctx := context.Background()
	pn, err := client.Store.LIDs.GetPNForLID(ctx, lid)
	if err != nil {
		return nil, err
	}

	if pn.IsEmpty() && data.GroupJid != "" {
		group, perr := types.ParseJID(data.GroupJid)
		if perr != nil || group.Server != types.GroupServer {
			return nil, ErrInvalidGroupJID
		}
		if _, gerr := client.GetGroupInfo(ctx, group); gerr != nil {
			u.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to refresh group %s for lid lookup: %v", instance.Id, group, gerr)
		} else if pn, err = client.Store.LIDs.GetPNForLID(ctx, lid); err != nil {
			return nil, err
		}
	}

	if pn.IsEmpty() {
		return nil, ErrLIDNotMapped
	}
	return &ResolveLidResult{Lid: lid.String(), PhoneNumber: pn.User, JID: pn.String()}, nil
}
