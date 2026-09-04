package database

import (
	"errors"
	"fmt"
	"sort"
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

func normalizePipelineIDs(pipelineIDs []int) []int {
	ids := append([]int(nil), pipelineIDs...)
	sort.Ints(ids)
	unique := ids[:0]
	for _, id := range ids {
		if id > 0 && (len(unique) == 0 || unique[len(unique)-1] != id) {
			unique = append(unique, id)
		}
	}
	return unique
}

func (db *DataBase) BanSubmission(pipelineID int, adminUserID uint, reason string) error {
	_, err := db.ModerateSubmissions([]int{pipelineID}, adminUserID, models.SubmissionModerationActionBan, reason)
	return err
}

func (db *DataBase) UnbanSubmission(pipelineID int, adminUserID uint, reason string) error {
	_, err := db.ModerateSubmissions([]int{pipelineID}, adminUserID, models.SubmissionModerationActionUnban, reason)
	return err
}

// ModerateSubmissions applies one moderation action atomically. Pipeline rows
// are locked in ID order so overlapping bulk requests cannot deadlock or create
// duplicate state transitions.
func (db *DataBase) ModerateSubmissions(pipelineIDs []int, adminUserID uint, action models.SubmissionModerationAction, reason string) (int, error) {
	ids := normalizePipelineIDs(pipelineIDs)
	if len(ids) == 0 {
		return 0, errors.New("no pipeline IDs")
	}
	if action != models.SubmissionModerationActionBan && action != models.SubmissionModerationActionUnban {
		return 0, fmt.Errorf("unsupported moderation action %q", action)
	}

	changed := 0
	err := db.Transaction(func(tx *gorm.DB) error {
		var lockedIDs []int
		if err := tx.Model(&models.Pipeline{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id IN ?", ids).
			Order("id").
			Pluck("id", &lockedIDs).Error; err != nil {
			return err
		}
		if len(lockedIDs) != len(ids) {
			return gorm.ErrRecordNotFound
		}

		var current []models.SubmissionBan
		if err := tx.Where("pipeline_id IN ?", ids).Find(&current).Error; err != nil {
			return err
		}
		banned := make(map[int]bool, len(current))
		for _, ban := range current {
			banned[ban.PipelineID] = true
		}

		now := time.Now()
		bansToCreate := make([]models.SubmissionBan, 0, len(ids))
		bansToDelete := make([]int, 0, len(ids))
		events := make([]models.SubmissionModerationEvent, 0, len(ids))
		for _, pipelineID := range ids {
			if action == models.SubmissionModerationActionBan && banned[pipelineID] {
				continue
			}
			if action == models.SubmissionModerationActionUnban && !banned[pipelineID] {
				continue
			}
			previousBanned := banned[pipelineID]
			currentBanned := action == models.SubmissionModerationActionBan
			if currentBanned {
				bansToCreate = append(bansToCreate, models.SubmissionBan{PipelineID: pipelineID, AdminUserID: adminUserID, Reason: reason, CreatedAt: now})
			} else {
				bansToDelete = append(bansToDelete, pipelineID)
			}
			events = append(events, models.SubmissionModerationEvent{
				PipelineID: pipelineID, AdminUserID: adminUserID, Action: action, Reason: reason,
				PreviousBanned: previousBanned, CurrentBanned: currentBanned, CreatedAt: now,
			})
		}
		if len(bansToCreate) > 0 {
			if err := tx.Create(&bansToCreate).Error; err != nil {
				return err
			}
		}
		if len(bansToDelete) > 0 {
			if err := tx.Delete(&models.SubmissionBan{}, "pipeline_id IN ?", bansToDelete).Error; err != nil {
				return err
			}
		}
		if len(events) > 0 {
			if err := tx.Create(&events).Error; err != nil {
				return err
			}
		}
		changed = len(events)
		return nil
	})
	return changed, err
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
