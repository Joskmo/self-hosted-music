package handlers

import (
	"encoding/json"
	"net/http"
	"os"

	"auth-panel/internal/session"
	"auth-panel/internal/upload"
)

func UploadPageHandler(w http.ResponseWriter, r *http.Request) {
	RenderTemplate(w, "upload.html", map[string]any{"NavidromeURL": PublicNavidromeURL()})
}

func UploadHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireNavidromeAuth(w, r, sessions) {
			return
		}

		musicDir := os.Getenv("MUSIC_DIR")
		if musicDir == "" {
			musicDir = "/music"
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			JSONError(w, "Parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		files := r.MultipartForm.File["files"]
		saved, err := upload.SaveUploads(musicDir, files)
		if err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": saved})
	}
}

func UploadZipHandler(sessions *session.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireNavidromeAuth(w, r, sessions) {
			return
		}

		musicDir := os.Getenv("MUSIC_DIR")
		if musicDir == "" {
			musicDir = "/music"
		}

		if err := r.ParseMultipartForm(128 << 20); err != nil {
			JSONError(w, "Parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		fh := r.MultipartForm.File["file"]
		if len(fh) == 0 {
			JSONError(w, "No file", http.StatusBadRequest)
			return
		}

		saved, err := upload.SaveUploadedZip(musicDir, fh[0])
		if err != nil {
			JSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]any{"ok": true, "saved": saved})
	}
}
