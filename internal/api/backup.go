package api

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"sonmi/internal/db"
)

func StartDBBackupTicker(dbConn db.Database) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("sonmi-backup-%d.db", time.Now().UTC().Unix()))

		if err := dbConn.Vacuum(tmpPath); err != nil {
			log.Printf("Error: failed to vacuum DB for backup: %v", err)
			continue
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			log.Printf("Error: failed to open backup file: %v", err)
			continue
		}

		key := fmt.Sprintf("backups/sonmi-%s.db", time.Now().UTC().Format(time.DateOnly))
		if _, err := UploadDailyPhoto(context.Background(), key, f); err != nil {
			log.Printf("Error: failed to upload DB backup to S3: %v", err)
		} else {
			log.Printf("DB backup uploaded successfully: %s", key)
		}

		f.Close()
		os.Remove(tmpPath)
	}
}
