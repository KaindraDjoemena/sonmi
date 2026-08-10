package api

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // support .jpegs only
	"log"
	"net/http"
	"os"
	"time"

	"sonmi/internal/db"
)

const (
	AUTH_HEADER_FIELD = "Authentication-Key"
)

type MediaServer struct {
	framePipe chan image.Image
	currFrame image.Image
	db        db.Database
}

func (ms *MediaServer) postFrame(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(r.Header, AUTH_HEADER_FIELD, os.Getenv("MEDIA_AUTH_KEY")) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	img, _, err := image.Decode(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ms.framePipe <- img

	fmt.Fprintln(w, "Image successfully piped to TUI", http.StatusOK)
}

type journalAPIResponse struct {
	Id              int       `json:"id"`
	DayRecap        string    `json:"day_recap"`
	PlanForTomorrow string    `json:"plan_for_tomorrow"`
	AgentMusings    string    `json:"agent_musings"`
	IsStale         bool      `json:"is_stale"`
	ValidForDate    string    `json:"valid_for_date"`
	ImgUrl          string    `json:"img_url"`
	Time            time.Time `json:"time"`
}

func (ms *MediaServer) getJournals(w http.ResponseWriter, r *http.Request) {
	if !isAuthorized(r.Header, AUTH_HEADER_FIELD, os.Getenv("MEDIA_AUTH_KEY")) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := ms.db.SelectLatestNJournalEntryRows(30)
	if err != nil {
		log.Printf("Database error fetching journals: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	res := make([]journalAPIResponse, 0, len(rows))
	for _, row := range rows {
		res = append(res, journalAPIResponse{
			Id:              row.Id,
			DayRecap:        row.DayRecap,
			PlanForTomorrow: row.PlanForTomorrow,
			AgentMusings:    row.AgentMusings,
			IsStale:         row.IsStale,
			ValidForDate:    row.ValidForDate,
			ImgUrl:          row.ImgUrl,
			Time:            row.Time,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		log.Printf("Error encoding journal response: %v", err)
	}
}

func StartHTTPServer(addr string, framePipe chan image.Image, dbConn db.Database) {
	rootMux := http.NewServeMux()

	s := &http.Server{
		Addr:           addr,
		Handler:        rootMux,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	log.Print("Initialized HTTP Server")
	log.Printf("Address: %s", s.Addr)
	log.Printf("Read Timeout: %s", s.ReadTimeout)
	log.Printf("Write Timeout: %s", s.WriteTimeout)
	log.Printf("Max Header Bytes: %d", s.MaxHeaderBytes)

	mediaServer := &MediaServer{
		framePipe: framePipe,
		db:        dbConn,
	}

	rootMux.Handle("POST /frame-stream", http.HandlerFunc(mediaServer.postFrame))
	rootMux.Handle("GET /api/journals", http.HandlerFunc(mediaServer.getJournals))

	log.Fatal(s.ListenAndServe())
}

// ///////////////////////// HELPERS /////////////////////////////////////////
func isAuthorized(header http.Header, field string, key string) bool {
	if key == "" {
		return false
	}

	if authKey := header.Get(field); authKey != key {
		return false
	}

	return true
}
