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

func TestDiscoverActionsHaveVisibleSpacing(t *testing.T) {
	css, err := webFiles.ReadFile("web/app.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".actions{display:flex;align-items:center;gap:8px;") {
		t.Fatal("SoundCloud result actions must use an 8px visual gap")
	}
}
func TestDiscoverTemplateOffersCollectionSearchModes(t *testing.T) {
	handlers.InitWebFS(webFiles, "web")
	rr := httptest.NewRecorder()
	handlers.RenderTemplate(rr, "discover.html", nil)
	body := rr.Body.String()
	for _, want := range []string{`data-mode="everything"`, `data-mode="tracks"`, `data-mode="albums"`, `data-mode="playlists"`, "Подтвердить загрузку", "mode=", "collection"} {
		if !strings.Contains(body, want) {
			t.Fatalf("discover template is missing collection search UI %q", want)
		}
	}
}

func TestDiscoverTemplateProvidesSoundCloudPreviewWithDownloadFallback(t *testing.T) {
	handlers.InitWebFS(webFiles, "web")
	rr := httptest.NewRecorder()
	handlers.RenderTemplate(rr, "discover.html", nil)
	body := rr.Body.String()
	for _, want := range []string{"w.soundcloud.com/player/api.js", "SC.Widget", "auto_play=true", "previewFallback"} {
		if !strings.Contains(body, want) {
			t.Fatalf("discover template is missing preview capability %q", want)
		}
	}
}
func TestAdminTemplateDoesNotInterpolateUserFieldsIntoHTML(t *testing.T) {
	handlers.InitWebFS(webFiles, "web")
	rr := httptest.NewRecorder()
	handlers.RenderTemplate(rr, "admin.html", map[string]any{"NavidromeURL": "https://music.example.test"})
	body := rr.Body.String()
	start := strings.Index(body, "async function loadUsers()")
	end := strings.Index(body, "async function toggleRole")
	if start < 0 || end < start {
		t.Fatal("admin user renderer was not found")
	}
	usersRenderer := body[start:end]
	for _, unsafe := range []string{"innerHTML", "setAttribute(", "insertAdjacentHTML", "outerHTML", "onclick="} {
		if strings.Contains(usersRenderer, unsafe) {
			t.Fatalf("admin user renderer must not build executable HTML from user fields: found %q", unsafe)
		}
	}
	for _, safe := range []string{"name.textContent = u.name || u.username", "username.textContent = u.username", "roleBtn.onclick = () => toggleRole(u.id, !u.is_admin)", "deleteBtn.onclick = () => deleteUser(u.id)"} {
		if !strings.Contains(usersRenderer, safe) {
			t.Fatalf("admin user renderer is missing safe DOM assignment %q", safe)
		}
	}
}
