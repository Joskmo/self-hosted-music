package metube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientAddAudioSubmitsFixedSafePayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/add"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.Method, http.MethodPost; got != want {
			t.Fatalf("method = %q, want %q", got, want)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if got, want := body["url"], "https://soundcloud.com/artist/track"; got != want {
			t.Fatalf("url = %v, want %v", got, want)
		}
		for field, want := range map[string]any{"download_type": "audio", "format": "mp3", "quality": "best", "auto_start": true, "ytdl_options_overrides": ""} {
			if got := body[field]; got != want {
				t.Fatalf("%s = %v, want %v", field, got, want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL, server.Client()).AddAudio(context.Background(), "https://soundcloud.com/artist/track"); err != nil {
		t.Fatal(err)
	}
}

func TestClientAddAudioAcceptsSoundCloudCollection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if got, want := body["url"], "https://soundcloud.com/gone-fludd/sets/fluddality"; got != want {
			t.Fatalf("url = %v, want %v", got, want)
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL, server.Client()).AddAudio(context.Background(), "https://soundcloud.com/gone-fludd/sets/fluddality"); err != nil {
		t.Fatal(err)
	}
}
func TestClientAddAudioRejectsRedirect(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	if err := NewClient(server.URL, server.Client()).AddAudio(context.Background(), "https://soundcloud.com/artist/track"); err == nil {
		t.Fatal("expected redirect to be rejected")
	}
	if redirected {
		t.Fatal("client followed redirect")
	}
}
func TestIsSoundCloudURLRejectsNonCanonicalURLs(t *testing.T) {
	for _, raw := range []string{
		"https://soundcloud.com:8443/artist/track",
		"https://soundcloud.com:/artist/track",
		"https://soundcloud.com/artist/track?utm_source=test",
		"https://soundcloud.com/artist/track#fragment",
	} {
		if isSoundCloudURL(raw) {
			t.Fatalf("expected non-canonical URL %q to be rejected", raw)
		}
	}
}
func TestClientAddAudioRejectsNonSoundCloudURL(t *testing.T) {
	if err := NewClient("http://metube", http.DefaultClient).AddAudio(context.Background(), "https://evil.example/track"); err == nil {
		t.Fatal("expected non-SoundCloud URL to be rejected")
	}
}
