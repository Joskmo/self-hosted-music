package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"auth-panel/internal/session"
)

func TestUploadPageHandlerRedirectsAnonymousUserToPrimaryLogin(t *testing.T) {
	sessions := session.New()
	req := httptest.NewRequest(http.MethodGet, "/upload", nil)
	rr := httptest.NewRecorder()

	UploadPageHandler(sessions)(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusFound)
	}
	if got, want := rr.Header().Get("Location"), "/login"; got != want {
		t.Fatalf("location = %q, want %q", got, want)
	}
}
