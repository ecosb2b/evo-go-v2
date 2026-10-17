package receipt_repository

import (
	"time"

	receipt_model "github.com/evolution-foundation/evolution-go/pkg/receipt/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReceiptRepository grava e lê o andamento das mensagens enviadas.
type ReceiptRepository interface {
	// Record marca o estado nas mensagens (cria a linha se não existir).
	// Cada data guarda a PRIMEIRA ocorrência; nada volta atrás.
	Record(instanceID string, messageIDs []string, chat string, state receipt_model.State, at time.Time) error
	// Find devolve os registros da instância para os ids pedidos.
	Find(instanceID string, messageIDs []string) ([]receipt_model.MessageReceipt, error)
	// DeleteOlderThan apaga o que não muda desde antes do corte.
	DeleteOlderThan(cutoff time.Time) (int64, error)
}

type receiptRepository struct {
	db *gorm.DB
}

func NewReceiptRepository(db *gorm.DB) ReceiptRepository {
	return &receiptRepository{db: db}
}

// LEAST ignora NULL no Postgres: fica a data mais antiga entre a gravada e a
// nova, então recibos fora de ordem ou repetidos não mexem no que já existe.
func earliest(column string) clause.Expr {
	return gorm.Expr("LEAST(message_receipts." + column + ", EXCLUDED." + column + ")")
}

func (r *receiptRepository) Record(instanceID string, messageIDs []string, chat string, state receipt_model.State, at time.Time) error {
	rows := make([]receipt_model.MessageReceipt, 0, len(messageIDs))
	seen := make(map[string]bool, len(messageIDs))
	for _, id := range messageIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		row := receipt_model.MessageReceipt{InstanceID: instanceID, MessageID: id, Chat: chat}
		receipt_model.Apply(&row, state, at)
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil
	}

	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "instance_id"}, {Name: "message_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"chat":         gorm.Expr("COALESCE(NULLIF(message_receipts.chat, ''), EXCLUDED.chat)"),
			"sent_at":      earliest("sent_at"),
			"delivered_at": earliest("delivered_at"),
			"read_at":      earliest("read_at"),
			"played_at":    earliest("played_at"),
			"updated_at":   gorm.Expr("EXCLUDED.updated_at"),
		}),
	}).Create(&rows).Error
}

func (r *receiptRepository) Find(instanceID string, messageIDs []string) ([]receipt_model.MessageReceipt, error) {
	var rows []receipt_model.MessageReceipt
	if len(messageIDs) == 0 {
		return rows, nil
	}
	err := r.db.Where("instance_id = ? AND message_id IN ?", instanceID, messageIDs).Find(&rows).Error
	return rows, err
}

func (r *receiptRepository) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result := r.db.Where("updated_at < ?", cutoff).Delete(&receipt_model.MessageReceipt{})
	return result.RowsAffected, result.Error
}
