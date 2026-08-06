package typebot_repository

import (
	typebot_model "github.com/evolution-foundation/evolution-go/pkg/typebot/model"
	"gorm.io/gorm"
)

type TypebotRepository interface {
	CreateBot(bot *typebot_model.Typebot) error
	UpdateBot(bot *typebot_model.Typebot) error
	DeleteBot(instanceID, id string) error
	GetBotByID(instanceID, id string) (*typebot_model.Typebot, error)
	GetBotsByInstanceID(instanceID string) ([]typebot_model.Typebot, error)
	// GetActiveBot devolve o bot que deve atender a instância, ou nil se não
	// houver nenhum habilitado.
	GetActiveBot(instanceID string) (*typebot_model.Typebot, error)

	CreateSession(session *typebot_model.TypebotSession) error
	UpdateSession(session *typebot_model.TypebotSession) error
	DeleteSession(instanceID, id string) error
	GetSessionByRemoteJid(instanceID, remoteJid string) (*typebot_model.TypebotSession, error)
	GetSessionsByInstanceID(instanceID string) ([]typebot_model.TypebotSession, error)
	SetSessionStatus(instanceID, id, status string) error
	// SetSessionStatusByRemoteJid muda o status pela chave que quem chama de fora
	// conhece: o contato. Devolve false quando não há sessão para aquele JID, de
	// modo que o chamador possa distinguir "encerrei" de "não havia nada" — sem
	// isso um webhook de proteção falharia em silêncio.
	SetSessionStatusByRemoteJid(instanceID, remoteJid, status string) (bool, error)
}

type typebotRepository struct {
	db *gorm.DB
}

func (t *typebotRepository) CreateBot(bot *typebot_model.Typebot) error {
	return t.db.Create(bot).Error
}

func (t *typebotRepository) UpdateBot(bot *typebot_model.Typebot) error {
	return t.db.Save(bot).Error
}

func (t *typebotRepository) DeleteBot(instanceID, id string) error {
	// As sessões são removidas junto: sem o bot elas não têm para onde
	// continuar, e deixá-las faria a próxima mensagem tentar um continueChat
	// contra um fluxo que não existe mais.
	return t.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("instance_id = ? AND typebot_id = ?", instanceID, id).
			Delete(&typebot_model.TypebotSession{}).Error; err != nil {
			return err
		}
		return tx.Where("instance_id = ? AND id = ?", instanceID, id).
			Delete(&typebot_model.Typebot{}).Error
	})
}

func (t *typebotRepository) GetBotByID(instanceID, id string) (*typebot_model.Typebot, error) {
	var bot typebot_model.Typebot
	err := t.db.Where("instance_id = ? AND id = ?", instanceID, id).First(&bot).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &bot, nil
}

func (t *typebotRepository) GetBotsByInstanceID(instanceID string) ([]typebot_model.Typebot, error) {
	var bots []typebot_model.Typebot
	err := t.db.Where("instance_id = ?", instanceID).Order("created_at asc").Find(&bots).Error
	if err != nil {
		return nil, err
	}
	return bots, nil
}

func (t *typebotRepository) GetActiveBot(instanceID string) (*typebot_model.Typebot, error) {
	var bot typebot_model.Typebot
	err := t.db.Where("instance_id = ? AND enabled = ?", instanceID, true).
		Order("created_at asc").First(&bot).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &bot, nil
}

func (t *typebotRepository) CreateSession(session *typebot_model.TypebotSession) error {
	return t.db.Create(session).Error
}

func (t *typebotRepository) UpdateSession(session *typebot_model.TypebotSession) error {
	return t.db.Save(session).Error
}

func (t *typebotRepository) DeleteSession(instanceID, id string) error {
	return t.db.Where("instance_id = ? AND id = ?", instanceID, id).
		Delete(&typebot_model.TypebotSession{}).Error
}

func (t *typebotRepository) GetSessionByRemoteJid(instanceID, remoteJid string) (*typebot_model.TypebotSession, error) {
	var session typebot_model.TypebotSession
	err := t.db.Where("instance_id = ? AND remote_jid = ?", instanceID, remoteJid).
		Order("updated_at desc").First(&session).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &session, nil
}

func (t *typebotRepository) GetSessionsByInstanceID(instanceID string) ([]typebot_model.TypebotSession, error) {
	var sessions []typebot_model.TypebotSession
	err := t.db.Where("instance_id = ?", instanceID).Order("updated_at desc").Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (t *typebotRepository) SetSessionStatus(instanceID, id, status string) error {
	return t.db.Model(&typebot_model.TypebotSession{}).
		Where("instance_id = ? AND id = ?", instanceID, id).
		Update("status", status).Error
}

func (t *typebotRepository) SetSessionStatusByRemoteJid(instanceID, remoteJid, status string) (bool, error) {
	result := t.db.Model(&typebot_model.TypebotSession{}).
		Where("instance_id = ? AND remote_jid = ?", instanceID, remoteJid).
		Update("status", status)

	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func NewTypebotRepository(db *gorm.DB) TypebotRepository {
	return &typebotRepository{db: db}
}
