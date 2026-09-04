package database

import (
	"time"

	"github.com/bigredeye/notmanytask/internal/models"
)

type AdminStatisticsSummary struct {
	ActiveBans          int64
	BanActionsLast7Days int64
	RepeatOffenders     int64
	AffectedTasks       int64
}

type AdminRepeatOffender struct {
	GitlabLogin string
	Name        string
	BanCount    int64
	ActiveBans  int64
}

type AdminModeratedTask struct {
	Task         string
	BanCount     int64
	ActiveBans   int64
	LastBannedAt time.Time
}

type AdminBenchmarkTrend struct {
	Task             string
	ReportsLast7     int64
	ReportsPrevious7 int64
	AverageLast7     float64
	AveragePrevious7 float64
	ChangePercent    float64
	Improved         bool
}

type AdminStatistics struct {
	Summary         AdminStatisticsSummary
	RepeatOffenders []AdminRepeatOffender
	ModeratedTasks  []AdminModeratedTask
	BenchmarkTrends []AdminBenchmarkTrend
}

func (db *DataBase) GetAdminStatistics() (*AdminStatistics, error) {
	statistics := &AdminStatistics{
		RepeatOffenders: make([]AdminRepeatOffender, 0),
		ModeratedTasks:  make([]AdminModeratedTask, 0),
		BenchmarkTrends: make([]AdminBenchmarkTrend, 0),
	}
	if err := db.Raw(`
        SELECT
            (SELECT COUNT(*) FROM submission_bans) AS active_bans,
            (SELECT COUNT(*) FROM submission_moderation_events
              WHERE action = ? AND created_at >= NOW() - INTERVAL '7 days') AS ban_actions_last7_days,
            (SELECT COUNT(*) FROM (
                SELECT u.id
                  FROM submission_moderation_events AS event
                  JOIN pipelines AS p ON p.id = event.pipeline_id
                  JOIN users AS u ON u.project_name = p.project AND u.deleted_at IS NULL
                 WHERE event.action = ?
                 GROUP BY u.id
                HAVING COUNT(*) >= 2
            ) AS repeated) AS repeat_offenders,
            (SELECT COUNT(DISTINCT p.task)
               FROM submission_moderation_events AS event
               JOIN pipelines AS p ON p.id = event.pipeline_id
              WHERE event.action = ?) AS affected_tasks
    `, models.SubmissionModerationActionBan, models.SubmissionModerationActionBan, models.SubmissionModerationActionBan).
		Scan(&statistics.Summary).Error; err != nil {
		return nil, err
	}

	if err := db.Raw(`
        SELECT COALESCE(u.gitlab_login, '') AS gitlab_login,
               CONCAT_WS(' ', u.first_name, u.last_name) AS name,
               COUNT(*) AS ban_count,
               COUNT(DISTINCT sb.pipeline_id) AS active_bans
          FROM submission_moderation_events AS event
          JOIN pipelines AS p ON p.id = event.pipeline_id
          JOIN users AS u ON u.project_name = p.project AND u.deleted_at IS NULL
          LEFT JOIN submission_bans AS sb ON sb.pipeline_id = p.id
         WHERE event.action = ?
         GROUP BY u.id, u.gitlab_login, u.first_name, u.last_name
        HAVING COUNT(*) >= 2
         ORDER BY ban_count DESC, gitlab_login
         LIMIT 10
    `, models.SubmissionModerationActionBan).Scan(&statistics.RepeatOffenders).Error; err != nil {
		return nil, err
	}

	if err := db.Raw(`
        SELECT p.task,
               COUNT(*) AS ban_count,
               COUNT(DISTINCT sb.pipeline_id) AS active_bans,
               MAX(event.created_at) AS last_banned_at
          FROM submission_moderation_events AS event
          JOIN pipelines AS p ON p.id = event.pipeline_id
          LEFT JOIN submission_bans AS sb ON sb.pipeline_id = p.id
         WHERE event.action = ?
         GROUP BY p.task
         ORDER BY ban_count DESC, p.task
         LIMIT 10
    `, models.SubmissionModerationActionBan).Scan(&statistics.ModeratedTasks).Error; err != nil {
		return nil, err
	}

	if err := db.Raw(`
        WITH task_trends AS (
            SELECT task,
                   COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days') AS reports_last7,
                   COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days'
                                      AND created_at < NOW() - INTERVAL '7 days') AS reports_previous7,
                   AVG(metric) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days') AS average_last7,
                   AVG(metric) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days'
                                        AND created_at < NOW() - INTERVAL '7 days') AS average_previous7
              FROM benchmark_results
             WHERE created_at >= NOW() - INTERVAL '14 days'
             GROUP BY task
        )
        SELECT task, reports_last7, reports_previous7,
               COALESCE(average_last7, 0) AS average_last7,
               COALESCE(average_previous7, 0) AS average_previous7,
               CASE WHEN average_previous7 IS NULL OR average_previous7 = 0 OR average_last7 IS NULL THEN 0
                    ELSE (average_last7 - average_previous7) / average_previous7 * 100 END AS change_percent,
               COALESCE(average_last7 < average_previous7, FALSE) AS improved
          FROM task_trends
         ORDER BY reports_last7 DESC, task
         LIMIT 10
    `).Scan(&statistics.BenchmarkTrends).Error; err != nil {
		return nil, err
	}
	return statistics, nil
}
