package main

import (
	"strings"
	"testing"
)

func TestUploadLoginPayloadClosesFetchOptions(t *testing.T) {
	template, err := webFiles.ReadFile("web/upload.html")
	if err != nil {
		t.Fatal(err)
	}

	const want = "JSON.stringify({username:username.value,password:password.value})}),j=await r.json()"
	if !strings.Contains(string(template), want) {
		t.Fatalf("upload login payload must close JSON.stringify, fetch options, and fetch call: missing %q", want)
	}
}
