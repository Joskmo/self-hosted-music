package soundcloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestClientSearchTracksUsesAPIv2AndPreservesCyrillic(t *testing.T) {
	const clientID = "0123456789abcdefghijklmnopqrstuv"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<script src="/assets/app.js"></script>`))
		case "/assets/app.js":
			_, _ = w.Write([]byte(`client_id: "` + clientID + `"`))
		case "/search/tracks":
			if got, want := r.URL.Query().Get("q"), "ДРИПСЭТ"; got != want {
				t.Fatalf("q = %q, want %q", got, want)
			}
			if got := r.URL.Query().Get("client_id"); got != clientID {
				t.Fatalf("client_id was not obtained from SoundCloud public assets")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"collection":[{"title":"ДРИПСЭТ","permalink_url":"https://soundcloud.com/gonefludd/dripset","user":{"username":"GONE.Fludd"}}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	results, err := NewClient(server.URL, server.Client()).Search(context.Background(), " ДРИПСЭТ ")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := results, []Track{{Title: "ДРИПСЭТ", Artist: "GONE.Fludd", URL: "https://soundcloud.com/gonefludd/dripset", Type: Tracks}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %#v, want %#v", got, want)
	}
}

func TestClientSearchAlbumsUsesAPIv2(t *testing.T) {
	const clientID = "0123456789abcdefghijklmnopqrstuv"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<script src="/assets/app.js"></script>`))
		case "/assets/app.js":
			_, _ = w.Write([]byte(`client_id: "` + clientID + `"`))
		case "/search/albums":
			if got, want := r.URL.Query().Get("q"), "gone.fludd"; got != want {
				t.Fatalf("q = %q, want %q", got, want)
			}
			if got := r.URL.Query().Get("client_id"); got != clientID {
				t.Fatalf("client_id was not obtained from SoundCloud public assets")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"collection":[{"title":"FLUDDALITY","permalink_url":"https://soundcloud.com/gone-fludd/sets/fluddality","user":{"username":"GONE.Fludd"}}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	results, err := NewClient(server.URL, server.Client()).Search(context.Background(), "gone.fludd", Albums)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := results, []Track{{Title: "FLUDDALITY", Artist: "GONE.Fludd", URL: "https://soundcloud.com/gone-fludd/sets/fluddality", Type: Albums}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %#v, want %#v", got, want)
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
