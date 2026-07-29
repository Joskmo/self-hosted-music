package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"auth-panel/internal/session"
	"auth-panel/internal/soundcloud"
)

type fakeSoundCloudSearcher struct {
	query string
}

func (f *fakeSoundCloudSearcher) Search(_ context.Context, query string) ([]soundcloud.Track, error) {
	f.query = query
	return []soundcloud.Track{{Title: "Track", Artist: "Artist", URL: "https://soundcloud.com/artist/track"}}, nil
}

func TestSoundCloudSearchHandlerReturnsResultsForAuthorizedSession(t *testing.T) {
	sessions := session.New()
	token := sessions.Create("member")
	searcher := &fakeSoundCloudSearcher{}
	req := httptest.NewRequest(http.MethodGet, "/api/discover/soundcloud?q=track", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	SoundCloudSearchHandler(sessions, searcher)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got, want := searcher.query, "track"; got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
	if body := rr.Body.String(); body != "[{\"title\":\"Track\",\"artist\":\"Artist\",\"url\":\"https://soundcloud.com/artist/track\"}]\n" {
		t.Fatalf("unexpected body: %s", body)
	}
}

type fakeSoundCloudEnqueuer struct {
	url string
}

func (f *fakeSoundCloudEnqueuer) AddAudio(_ context.Context, url string) error {
	f.url = url
	return nil
}

func TestSoundCloudAddHandlerQueuesResultForAuthorizedSession(t *testing.T) {
	sessions := session.New()
	token := sessions.Create("member")
	enqueuer := &fakeSoundCloudEnqueuer{}
	req := httptest.NewRequest(http.MethodPost, "/api/discover/soundcloud/add", strings.NewReader(`{"url":"https://soundcloud.com/artist/track"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	SoundCloudAddHandler(sessions, enqueuer, "http://example.com")(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusAccepted)
	}
	if got, want := enqueuer.url, "https://soundcloud.com/artist/track"; got != want {
		t.Fatalf("queued URL = %q, want %q", got, want)
	}
}

func TestRequireSameOriginRejectsSameHostDifferentScheme(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://music.example.test/api/discover/soundcloud/add", nil)
	req.Header.Set("Origin", "http://music.example.test")
	rr := httptest.NewRecorder()
	if requireSameOrigin(rr, req, "https://music.example.test") {
		t.Fatal("origin with a different scheme must be rejected")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}
func TestSoundCloudAddHandlerRejectsCrossOriginRequest(t *testing.T) {
	sessions := session.New()
	token := sessions.Create("member")
	enqueuer := &fakeSoundCloudEnqueuer{}
	req := httptest.NewRequest(http.MethodPost, "https://music.example.test/api/discover/soundcloud/add", strings.NewReader(`{"url":"https://soundcloud.com/artist/track"}`))
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	SoundCloudAddHandler(sessions, enqueuer, "http://example.com")(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if enqueuer.url != "" {
		t.Fatal("cross-origin request must not enqueue a download")
	}
}
func TestSoundCloudAddHandlerRejectsTrailingJSON(t *testing.T) {
	sessions := session.New()
	token := sessions.Create("member")
	enqueuer := &fakeSoundCloudEnqueuer{}
	req := httptest.NewRequest(http.MethodPost, "/api/discover/soundcloud/add", strings.NewReader(`{"url":"https://soundcloud.com/artist/track"}{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://example.com")
	req.AddCookie(&http.Cookie{Name: "session", Value: token})
	rr := httptest.NewRecorder()

	SoundCloudAddHandler(sessions, enqueuer, "http://example.com")(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if enqueuer.url != "" {
		t.Fatal("malformed JSON must not enqueue a download")
	}
}

func TestSoundCloudSearchHandlerRejectsAnonymousRequest(t *testing.T) {
	sessions := session.New()
	req := httptest.NewRequest(http.MethodGet, "/api/discover/soundcloud?q=track", nil)
	rr := httptest.NewRecorder()

	SoundCloudSearchHandler(sessions, &fakeSoundCloudSearcher{})(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
