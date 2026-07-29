package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"

	"auth-panel/internal/session"
	"auth-panel/internal/soundcloud"
)

type SoundCloudSearcher interface {
	Search(context.Context, string) ([]soundcloud.Track, error)
}

type SoundCloudEnqueuer interface {
	AddAudio(context.Context, string) error
}

func SoundCloudDiscoverPageHandler(w http.ResponseWriter, r *http.Request, sessions *session.Store) {
	if RequireSession(w, r, sessions) == "" {
		return
	}
	RenderTemplate(w, "discover.html", nil)
}

func requireSameOrigin(w http.ResponseWriter, r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.Host != r.Host || (origin.Scheme != "https" && origin.Scheme != "http") {
		JSONError(w, "недопустимый источник запроса", http.StatusForbidden)
		return false
	}
	return true
}

func SoundCloudAddHandler(sessions *session.Store, enqueuer SoundCloudEnqueuer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireNavidromeAuth(w, r, sessions) {
			return
		}
		if !requireSameOrigin(w, r) {
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			JSONError(w, "некорректный запрос", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			JSONError(w, "некорректный запрос", http.StatusBadRequest)
			return
		}
		if err := enqueuer.AddAudio(r.Context(), body.URL); err != nil {
			log.Printf("soundcloud enqueue: %v", err)
			JSONError(w, "не удалось добавить трек в очередь", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}
}
func SoundCloudSearchHandler(sessions *session.Store, searcher SoundCloudSearcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !RequireNavidromeAuth(w, r, sessions) {
			return
		}
		results, err := searcher.Search(r.Context(), r.URL.Query().Get("q"))
		if err != nil {
			log.Printf("soundcloud search: %v", err)
			JSONError(w, "поиск SoundCloud сейчас недоступен", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results)
	}
}
