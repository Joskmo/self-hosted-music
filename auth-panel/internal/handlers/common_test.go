package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestStaticHandlerServesEmbeddedCSS(t *testing.T) {
	original := webFS
	t.Cleanup(func() { webFS = original })
	webFS = fstest.MapFS{
		"app.css": &fstest.MapFile{Data: []byte("body{color:lime}")},
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	res := httptest.NewRecorder()
	http.StripPrefix("/assets/", StaticHandler()).ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Body.String(); got != "body{color:lime}" {
		t.Fatalf("body = %q", got)
	}
	if got := res.Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestStaticHandlerDoesNotEscapeEmbeddedFS(t *testing.T) {
	original := webFS
	t.Cleanup(func() { webFS = original })
	webFS = fstest.MapFS{"app.css": &fstest.MapFile{Data: []byte("ok")}}

	req := httptest.NewRequest(http.MethodGet, "/assets/../secret", nil)
	res := httptest.NewRecorder()
	http.StripPrefix("/assets/", StaticHandler()).ServeHTTP(res, req)

	if res.Code == http.StatusOK {
		t.Fatal("unexpected successful response for escaped path")
	}
	if !fs.ValidPath("app.css") {
		t.Fatal("test fixture must be a valid fs path")
	}
}
