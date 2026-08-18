package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/infrai-examples/course-asset-uploads/internal/assets"
)

const bucketName = "edtech-course-assets"

type server struct {
	uploads *assets.UploadService
	storage *assets.InfraiClient
}

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	storage := assets.NewInfraiClient(apiKey)
	s := &server{
		uploads: assets.NewUploadService(bucketName, storage, time.Now),
		storage: storage,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /uploads", s.createUpload)
	mux.HandleFunc("GET /uploads/status", s.uploadStatus)
	mux.HandleFunc("GET /educator/report", s.educatorReport)

	addr := envOr("ADDR", ":8080")
	log.Printf("course asset service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (s *server) createUpload(w http.ResponseWriter, r *http.Request) {
	var input assets.UploadRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	decision, grant, err := s.uploads.Authorize(r.Context(), input)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if !decision.Accepted {
		writeJSON(w, http.StatusUnprocessableEntity, decision)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		Decision assets.UploadDecision `json:"decision"`
		Grant    assets.UploadGrant    `json:"grant"`
	}{decision, grant})
}

func (s *server) uploadStatus(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "key is required"})
		return
	}
	found, err := s.storage.AssetUploaded(r.Context(), bucketName, key)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	state := "awaiting_upload"
	if found {
		state = "delivered"
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "found": found, "delivery_state": state})
}

func (s *server) educatorReport(w http.ResponseWriter, r *http.Request) {
	items, err := s.storage.ListCourseAssets(r.Context(), bucketName)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"uploaded_asset_count": len(items), "items": items})
}

func writeServiceError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	var infraiErr *assets.InfraiError
	if errors.As(err, &infraiErr) && infraiErr.HTTPStatus >= 400 && infraiErr.HTTPStatus < 500 {
		status = infraiErr.HTTPStatus
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
