package message_repository

import (
	message_model "github.com/evolution-foundation/evolution-go/pkg/message/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MessageRepository interface {
	InsertMessage(message message_model.Message) error
	GetMessageByID(messageID string) (*message_model.Message, error)
	DeleteAllMessages() (int64, error)
	GetLatestMessageID(source string) (string, string, error)
	GetStats() (*MessageStats, error)
}

// StatKV é um par rótulo/contagem usado nas agregações do dashboard.
type StatKV struct {
	Key   string `json:"key" gorm:"column:label"`
	Count int64  `json:"count" gorm:"column:total"`
}

// MessageStats agrega os dados da tabela de mensagens para o dashboard.
type MessageStats struct {
	Total      int64    `json:"total"`
	ByStatus   []StatKV `json:"byStatus"`
	ByDay      []StatKV `json:"byDay"`
	TopSources []StatKV `json:"topSources"`
}

type messageRepository struct {
	db *gorm.DB
}

func messageUpdateColumns(message message_model.Message) []string {
	updates := []string{"timestamp", "status", "source"}
	if len(message.Referral) > 0 {
		updates = append(updates, "referral")
	}

	return updates
}

func (m *messageRepository) InsertMessage(message message_model.Message) error {
	return m.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.AssignmentColumns(messageUpdateColumns(message)),
	}).Create(&message).Error
}

func (m *messageRepository) GetMessageByID(messageID string) (*message_model.Message, error) {
	var message message_model.Message
	err := m.db.Where("message_id = ?", messageID).First(&message).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &message, nil
}

func (m *messageRepository) DeleteAllMessages() (int64, error) {
	result := m.db.Exec("DELETE FROM messages")
	return result.RowsAffected, result.Error
}

func (m *messageRepository) GetLatestMessageID(source string) (string, string, error) {
	var message message_model.Message
	err := m.db.Where("source = ?", source).Order("timestamp DESC").First(&message).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", "", nil
		}
		return "", "", err
	}

	return message.MessageID, message.Timestamp, nil
}

// GetStats agrega a tabela de mensagens: total, quebra por status, por dia
// (últimos 14 dias, mais recente primeiro) e top contatos por volume.
// Observação: só mensagens recebidas são persistidas hoje (Status="Received",
// Source=número do contato), então byStatus tende a ser dominado por "Received".
func (m *messageRepository) GetStats() (*MessageStats, error) {
	stats := &MessageStats{ByStatus: []StatKV{}, ByDay: []StatKV{}, TopSources: []StatKV{}}

	if err := m.db.Model(&message_model.Message{}).Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	m.db.Model(&message_model.Message{}).
		Select("status as label, count(*) as total").
		Group("status").Order("total desc").
		Scan(&stats.ByStatus)

	m.db.Model(&message_model.Message{}).
		Select(`substr("timestamp", 1, 10) as label, count(*) as total`).
		Group(`substr("timestamp", 1, 10)`).Order("label desc").
		Limit(14).
		Scan(&stats.ByDay)

	m.db.Model(&message_model.Message{}).
		Select("source as label, count(*) as total").
		Group("source").Order("total desc").
		Limit(8).
		Scan(&stats.TopSources)

	return stats, nil
}

func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}
