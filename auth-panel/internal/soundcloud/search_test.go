package soundcloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSearchReturnsPublicTrackResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/search/sounds"; got != want {
			t.Fatalf("path = %q, want %q", got, want)
		}
		if got, want := r.URL.Query().Get("q"), "fred again"; got != want {
			t.Fatalf("q = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><main><li><a href="/fredagain/track-one">Track &amp; One</a><a href="/fredagain">Fred again..</a><span>3:42</span></li><li><a href="https://evil.example/track">Ignore this</a></li></main>`))
	}))
	defer server.Close()

	client := NewClient(server.URL, server.Client())
	results, err := client.Search(context.Background(), " fred again ")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one result", results)
	}
	if got, want := results[0], (Track{Title: "Track & One", Artist: "Fred again..", URL: "https://soundcloud.com/fredagain/track-one"}); got != want {
		t.Fatalf("result = %#v, want %#v", got, want)
	}
}

func TestClientSearchParsesNoScriptFallbackResults(t *testing.T) {
	page := `<noscript><ul><li><h2><a href="/fredagain/turn-on-the-lights-again">Track</a></h2></li></ul></noscript>`
	results := parseTracks(page)
	if len(results) != 1 {
		t.Fatalf("results = %#v, want one result", results)
	}
	if got, want := results[0].Artist, "fredagain"; got != want {
		t.Fatalf("artist = %q, want %q", got, want)
	}
}
func TestClientSearchRejectsEmptyQuery(t *testing.T) {
	client := NewClient("https://soundcloud.com", http.DefaultClient)
	if _, err := client.Search(context.Background(), " 	 "); err == nil {
		t.Fatal("expected an error for an empty query")
	}
}

func TestClientSearchRejectsRedirect(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	_, err := NewClient(server.URL, server.Client()).Search(context.Background(), "track")
	if err == nil {
		t.Fatal("expected redirect to be rejected")
	}
	if redirected {
		t.Fatal("client followed redirect")
	}
}
