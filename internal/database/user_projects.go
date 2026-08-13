package database

import "gorm.io/gorm"

func backfillUserProjectNames(db *gorm.DB) error {
	return db.Exec(`
        UPDATE users
           SET project_name = regexp_replace(rtrim(repository, '/'), '^.*/', '')
         WHERE COALESCE(project_name, '') = ''
           AND repository IS NOT NULL
           AND rtrim(repository, '/') <> ''
    `).Error
}
