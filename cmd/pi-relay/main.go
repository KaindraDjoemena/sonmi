package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

const (
	AUTH_HEADER_FIELD = "Authentication-Key"
)

func main() {
	if err := godotenv.Load(".env.pi-relay"); err != nil {
		log.Printf("Warning: Could not load .env.pi-relay: %v (relying on system env vars)", err)
	}

	listenAddr := os.Getenv("RELAY_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = "192.168.4.1:8080"
	}

	upstreamURL := os.Getenv("RELAY_UPSTREAM_URL")
	if upstreamURL == "" {
		log.Fatal("RELAY_UPSTREAM_URL must be set (e.g. http://100.84.119.64:8080/frame-stream)")
	}

	client := &http.Client{Timeout: 10 * time.Second}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /frame-stream", func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequest(http.MethodPost, upstreamURL, r.Body)
		if err != nil {
			log.Printf("Error building upstream request: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		req.Header.Set(AUTH_HEADER_FIELD, r.Header.Get(AUTH_HEADER_FIELD))
		req.ContentLength = r.ContentLength

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error forwarding to upstream: %v", err)
			http.Error(w, "upstream unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("Error copying upstream response: %v", err)
		}
	})

	s := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("pi-relay listening on %s, forwarding to %s", listenAddr, upstreamURL)
	log.Fatal(s.ListenAndServe())
}
