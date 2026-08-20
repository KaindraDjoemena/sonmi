package api

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"log"
	"sync"
	"time"

	"sonmi/internal/db"
)

func StartDailySnapshotTicker(framePipe <-chan image.Image, database db.Database) {
	var (
		mu          sync.Mutex
		latestFrame image.Image
	)

	// Start a goroutine to keep track of the latest frame
	go func() {
		for frame := range framePipe {
			mu.Lock()
			latestFrame = frame
			mu.Unlock()
		}
	}()

	for {
		// Calculate time until exactly 23:55 UTC (just before Journal Loop at 23:59)
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day(), 23, 55, 0, 0, time.UTC)
		if !next.After(now) {
			next = next.AddDate(0, 0, 1)
		}

		<-time.After(time.Until(next))

		mu.Lock()
		frame := latestFrame
		mu.Unlock()

		if frame == nil {
			log.Println("Warning: no frame available for daily snapshot, skipping")
			continue
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, frame, &jpeg.Options{Quality: 85}); err != nil {
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
