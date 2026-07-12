package database

import (
	"errors"
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SubmissionModerationEvent struct {
	ID             uint
	PipelineID     int
	AdminUserID    uint
	AdminName      string
	Action         models.SubmissionModerationAction
	Reason         string
	PreviousBanned bool
	CurrentBanned  bool
	CreatedAt      time.Time
}

func backfillSubmissionModerationEvents(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Serialize startup backfills across multiple application instances.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext('notmanytask:moderation-events-backfill'))").Error; err != nil {
			return err
		}
		return tx.Exec(`
            INSERT INTO submission_moderation_events
                (pipeline_id, admin_user_id, action, reason, previous_banned, current_banned, created_at)
            SELECT sb.pipeline_id, sb.admin_user_id, ?, sb.reason, FALSE, TRUE, sb.created_at
              FROM submission_bans AS sb
             WHERE NOT EXISTS (
                   SELECT 1
                     FROM submission_moderation_events AS event
                    WHERE event.pipeline_id = sb.pipeline_id
             )
        `, models.SubmissionModerationActionBan).Error
	})
}

func lockPipeline(tx *gorm.DB, pipelineID int) error {
	var pipeline models.Pipeline
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&pipeline, "id = ?", pipelineID).Error
}

func (db *DataBase) BanSubmission(pipelineID int, adminUserID uint, reason string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := lockPipeline(tx, pipelineID); err != nil {
			return err
		}
		var existing models.SubmissionBan
		err := tx.First(&existing, "pipeline_id = ?", pipelineID).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		now := time.Now()
		ban := &models.SubmissionBan{PipelineID: pipelineID, AdminUserID: adminUserID, Reason: reason, CreatedAt: now}
		if err := tx.Create(ban).Error; err != nil {
			return err
		}
		return tx.Create(&models.SubmissionModerationEvent{
			PipelineID:     pipelineID,
			AdminUserID:    adminUserID,
			Action:         models.SubmissionModerationActionBan,
			Reason:         reason,
			PreviousBanned: false,
			CurrentBanned:  true,
			CreatedAt:      now,
		}).Error
	})
}

func (db *DataBase) UnbanSubmission(pipelineID int, adminUserID uint, reason string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := lockPipeline(tx, pipelineID); err != nil {
			return err
		}
		var existing models.SubmissionBan
		err := tx.First(&existing, "pipeline_id = ?", pipelineID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Delete(&models.SubmissionBan{}, "pipeline_id = ?", pipelineID).Error; err != nil {
			return err
		}
		return tx.Create(&models.SubmissionModerationEvent{
			PipelineID:     pipelineID,
			AdminUserID:    adminUserID,
			Action:         models.SubmissionModerationActionUnban,
			Reason:         reason,
			PreviousBanned: true,
			CurrentBanned:  false,
			CreatedAt:      now,
		}).Error
	})
}

func (db *DataBase) ListSubmissionModerationEvents(pipelineIDs []int) ([]SubmissionModerationEvent, error) {
	events := make([]SubmissionModerationEvent, 0)
	if len(pipelineIDs) == 0 {
		return events, nil
	}
	err := db.Table("submission_moderation_events AS event").
		Select(`
            event.id,
            event.pipeline_id,
            event.admin_user_id,
            CONCAT_WS(' ', admin.first_name, admin.last_name) AS admin_name,
            event.action,
            event.reason,
            event.previous_banned,
            event.current_banned,
            event.created_at
        `).
		Joins("LEFT JOIN users AS admin ON admin.id = event.admin_user_id").
		Where("event.pipeline_id IN ?", pipelineIDs).
		Order("event.created_at DESC, event.id DESC").
		Scan(&events).Error
	return events, err
}
