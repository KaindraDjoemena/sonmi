package api

import (
	"log"
	"net/http"
	"os"
	"time"
)

// FireRevalidateWebhook POSTs to REVALIDATE_WEBHOOK_URL (if set) to trigger
// on-demand ISR revalidation on the Next.js devlog. Retries up to 3 times
// with backoff (30s, 60s, 120s). Never blocks — call as a goroutine.
// A missing or empty REVALIDATE_WEBHOOK_URL is a no-op (local dev safe).
func FireRevalidateWebhook() {
	url := os.Getenv("REVALIDATE_WEBHOOK_URL")
	if url == "" {
		return
	}

	delays := []time.Duration{0, 30 * time.Second, 60 * time.Second, 120 * time.Second}
	client := &http.Client{Timeout: 10 * time.Second}

	for i, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		resp, err := client.Post(url, "application/json", nil)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			resp.Body.Close()
			log.Printf("Revalidation webhook fired successfully (attempt %d)", i+1)
			return
		}
		if err != nil {
			log.Printf("Revalidation webhook attempt %d failed: %v", i+1, err)
		} else {
			log.Printf("Revalidation webhook attempt %d returned %d", i+1, resp.StatusCode)
			resp.Body.Close()
		}
	}
	log.Printf("Warning: revalidation webhook failed after all attempts")
}
