package api

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"time"

	"sonmi/internal/db"
)

func StartDailySnapshotTicker(framePipe <-chan image.Image, database db.Database) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	var latestFrame image.Image

	for {
		select {
		case frame := <-framePipe:
			latestFrame = frame
		case <-ticker.C:
			if latestFrame == nil {
				log.Println("Warning: no frame available for daily snapshot, skipping")
				continue
			}

			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, latestFrame, &jpeg.Options{Quality: 85}); err != nil {
				log.Printf("Error: failed to JPEG-encode frame: %v", err)
				continue
			}

			today := time.Now().UTC()
			fileKey := fmt.Sprintf("daily/%s.jpg", today.Format(time.DateOnly))

			url, err := UploadDailyPhoto(context.Background(), fileKey, &buf)
			if err != nil {
				log.Printf("Error: failed to upload daily photo to S3: %v", err)
				continue
			}

			if err := (db.DailyPhotoRow{Date: today.Format(time.DateOnly), ImgUrl: url, Time: today}).Insert(database); err != nil {
				log.Printf("Error: failed to insert daily photo row into DB: %v", err)
			} else {
				log.Printf("Daily snapshot saved successfully: %s", url)
			}
		}
	}
}
