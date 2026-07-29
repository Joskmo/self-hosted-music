package main

import (
	"os"
	"strings"
	"testing"
)

func TestAdminPageUsesSharedWorkspaceNavigation(t *testing.T) {
	page, err := os.ReadFile("web/admin.html")
	if err != nil {
		t.Fatal(err)
	}
	content := string(page)
	for _, required := range []string{
		`href="/assets/app.css"`, `src="/assets/app.js"`, `data-theme-toggle`,
		`href="/upload"`, `href="/discover"`, `href="/metube"`,
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("admin page is missing shared navigation element %q", required)
		}
	}
	if strings.Contains(content, "body { font-family: system-ui") {
		t.Fatal("admin page still has the legacy standalone stylesheet")
	}
}
