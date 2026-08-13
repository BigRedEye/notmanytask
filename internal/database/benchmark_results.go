package database

import "gorm.io/gorm"

func migrateBenchmarkResults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext('notmanytask:benchmark-results-migration'))").Error; err != nil {
			return err
		}
		if err := tx.Exec(`
            DELETE FROM benchmark_results AS older
             USING benchmark_results AS newer
             WHERE older.pipeline_id = newer.pipeline_id
               AND (older.created_at < newer.created_at
                    OR (older.created_at = newer.created_at AND older.id < newer.id))
        `).Error; err != nil {
			return err
		}
		return tx.Exec(`
            CREATE UNIQUE INDEX IF NOT EXISTS idx_benchmark_results_pipeline_id_unique
                ON benchmark_results (pipeline_id)
        `).Error
	})
}
