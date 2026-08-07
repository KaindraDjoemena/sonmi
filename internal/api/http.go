package api

import (
	"fmt"
	"image"
	_ "image/jpeg" // support .jpegs only
	"log"
	"net/http"
	"os"
	"time"
)

const (
	AUTH_HEADER_FIELD = "Authentication-Key"
)

type MediaServer struct {
	framePipe chan image.Image
	currFrame image.Image
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

func StartHTTPServer(addr string, framePipe chan image.Image) {
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
	}

	rootMux.Handle("POST /frame-stream", http.HandlerFunc(mediaServer.postFrame))

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
