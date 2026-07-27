package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"auth-panel/internal/handlers"
)

func TestMusicServerLinksUsePublicConfiguredURL(t *testing.T) {
	handlers.InitWebFS(webFiles, "web")
	const publicURL = "https://music.example.test"

	for _, template := range []string{"upload.html", "admin.html"} {
		t.Run(template, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handlers.RenderTemplate(rr, template, map[string]any{"NavidromeURL": publicURL})

			body := rr.Body.String()
			if !strings.Contains(body, `href="`+publicURL+`"`) {
				t.Fatalf("expected public Navidrome link in rendered %s, got: %s", template, body)
			}
			if strings.Contains(body, "localhost:4533") {
				t.Fatalf("rendered %s must not expose a server-local Navidrome URL", template)
			}
		})
	}
}
