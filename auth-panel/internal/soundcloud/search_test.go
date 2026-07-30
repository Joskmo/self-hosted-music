package soundcloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
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
			_, _ = w.Write([]byte(`{"collection":[{"id":42,"title":"ДРИПСЭТ","permalink_url":"https://soundcloud.com/gonefludd/dripset","duration":195000,"description":"official upload","artwork_url":"https://i1.sndcdn.com/art.jpg","genre":"hip-hop","publisher_metadata":{"artist":"GONE.Fludd","album_title":"SUPERNOVA","isrc":"RUA1D1234567"},"user":{"username":"GONE.Fludd"}}]}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	results, err := NewClient(server.URL, server.Client()).Search(context.Background(), " ДРИПСЭТ ")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := results, []Track{{Title: "ДРИПСЭТ", Artist: "GONE.Fludd", URL: "https://soundcloud.com/gonefludd/dripset", Type: Tracks, SourceID: 42, DurationMS: 195000, Description: "official upload", ArtworkURL: "https://i1.sndcdn.com/art.jpg", Genre: "hip-hop", Album: "SUPERNOVA", ISRC: "RUA1D1234567"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("results = %#v, want %#v", got, want)
	}
	encoded, err := json.Marshal(results[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"source_id":42`, `"duration_ms":195000`, `"isrc":"RUA1D1234567"`, `"album":"SUPERNOVA"`, `"description":"official upload"`} {
		if !strings.Contains(string(encoded), want) {
			t.Errorf("result JSON %s does not retain SoundCloud metadata %s", encoded, want)
		}
	}
}

func TestClientResolveGetsExactSoundCloudTrackMetadata(t *testing.T) {
	const clientID = "0123456789abcdefghijklmnopqrstuv"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<script src="/assets/app.js"></script>`))
		case "/assets/app.js":
			_, _ = w.Write([]byte(`client_id: "` + clientID + `"`))
		case "/resolve":
			if got, want := r.URL.Query().Get("url"), "https://soundcloud.com/gonefludd/dripset"; got != want {
				t.Fatalf("url = %q, want %q", got, want)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"title":"ДРИПСЭТ","permalink_url":"https://soundcloud.com/gonefludd/dripset","duration":195000,"publisher_metadata":{"artist":"GONE.Fludd","album_title":"SUPERNOVA","isrc":"RUA1D1234567"},"user":{"username":"wrong-uploader"}}`))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	track, err := NewClient(server.URL, server.Client()).Resolve(context.Background(), "https://soundcloud.com/gonefludd/dripset")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := track, (Track{Title: "ДРИПСЭТ", Artist: "GONE.Fludd", URL: "https://soundcloud.com/gonefludd/dripset", Type: Tracks, SourceID: 42, DurationMS: 195000, Album: "SUPERNOVA", ISRC: "RUA1D1234567"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("track = %#v, want %#v", got, want)
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
