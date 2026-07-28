package upload

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var runUnzip = func(zipPath, destination string) error {
	return exec.Command("unzip", "-o", zipPath, "-d", destination).Run()
}

func SaveUploads(musicDir string, files []*multipart.FileHeader) ([]string, error) {
	var saved []string
	for _, fh := range files {
		if !isAudioFile(fh.Filename) {
			continue
		}

		dest := filepath.Join(musicDir, filepath.Base(fh.Filename))
		if err := copyMultipartFile(fh, dest); err != nil {
			return saved, fmt.Errorf("save %s: %w", fh.Filename, err)
		}
		saved = append(saved, fh.Filename)
	}
	return saved, nil
}

func SaveUploadedZip(musicDir string, fh *multipart.FileHeader) ([]string, error) {
	src, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()

	tmpDir, err := os.MkdirTemp("", "upload-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	tmpZip := filepath.Join(tmpDir, "upload.zip")
	dst, err := os.Create(tmpZip)
	if err != nil {
		return nil, err
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return nil, err
	}
	dst.Close()

	if err := runUnzip(tmpZip, tmpDir); err != nil {
		return nil, fmt.Errorf("unzip: %w", err)
	}

	var saved []string
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if !isAudioFile(info.Name()) {
			return nil
		}

		rel, _ := filepath.Rel(tmpDir, path)
		destDir := filepath.Join(musicDir, filepath.Dir(rel))
		os.MkdirAll(destDir, 0755)

		dest := filepath.Join(musicDir, rel)
		if err := copyFile(path, dest); err != nil {
			return err
		}
		saved = append(saved, rel)
		return nil
	})

	return saved, err
}

func isAudioFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	audioExts := map[string]bool{
		".mp3":  true,
		".flac": true,
		".wav":  true,
		".ogg":  true,
		".m4a":  true,
		".aac":  true,
		".wma":  true,
		".opus": true,
	}
	return audioExts[ext]
}

func copyMultipartFile(fh *multipart.FileHeader, dest string) error {
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer src.Close()
	return copyToNewFile(src, dest)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return copyToNewFile(in, dst)
}

// copyToNewFile intentionally refuses collisions. Uploading must never replace
// an existing library file (or follow an existing symlink) without review.
func copyToNewFile(src io.Reader, dest string) error {
	dst, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(dest)
		return err
	}
	return nil
}
