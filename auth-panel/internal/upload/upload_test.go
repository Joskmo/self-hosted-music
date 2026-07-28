package upload

import (
	"archive/zip"
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveUploadedZipSkipsSymlinkedAudio(t *testing.T) {
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	h := &zip.FileHeader{Name: "secret.mp3"}
	h.SetMode(os.ModeSymlink | 0o777)
	w, err := zw.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("/etc/hostname")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "library.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(archive.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/upload", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if err := req.ParseMultipartForm(int64(body.Len())); err != nil {
		t.Fatal(err)
	}
	fh := req.MultipartForm.File["file"][0]

	musicDir := t.TempDir()
	oldRunUnzip := runUnzip
	runUnzip = func(_ string, destination string) error {
		return os.Symlink("/etc/hostname", filepath.Join(destination, "secret.mp3"))
	}
	t.Cleanup(func() { runUnzip = oldRunUnzip })
	saved, err := SaveUploadedZip(musicDir, fh)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 0 {
		t.Fatalf("saved = %v, want no symlinked files", saved)
	}
	if _, err := os.Stat(filepath.Join(musicDir, "secret.mp3")); !os.IsNotExist(err) {
		t.Fatalf("symlinked audio was copied: %v", err)
	}
}

func TestSaveUploadsDoesNotOverwriteExistingAudio(t *testing.T) {
	musicDir := t.TempDir()
	destination := filepath.Join(musicDir, "song.mp3")
	if err := os.WriteFile(destination, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", "song.mp3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("replacement")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatal(err)
	}

	_, err = SaveUploads(musicDir, request.MultipartForm.File["files"])
	if err == nil {
		t.Fatal("SaveUploads() overwrote an existing library file")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Fatalf("existing file = %q, want original content", got)
	}
}
