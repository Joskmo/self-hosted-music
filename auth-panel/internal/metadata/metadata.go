// Package metadata discovers completed audio files and evaluates matching safety.
package metadata

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Track is the source information obtained from tags or a filename.
type Track struct {
	Title  string
	Artist string
	Album  string
}

// Candidate is a public-catalogue match candidate.
type Candidate struct {
	ID     string
	Title  string
	Artist string
	Album  string
}

var audioExtensions = map[string]bool{
	".aac": true, ".flac": true, ".m4a": true, ".mp3": true,
	".ogg": true, ".opus": true, ".wav": true, ".wma": true,
}

// fileStabilityWindow gives downloaders that write directly to their final name
// time to finish before the scanner reads the audio file.
const fileStabilityWindow = 2 * time.Minute

// DiscoverAudioFiles returns paths relative to root for completed supported files.
// Hidden and common downloader temporary files are deliberately left untouched.
func DiscoverAudioFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := info.Name()
		if info.Mode()&os.ModeSymlink != 0 || strings.HasPrefix(base, ".") || strings.HasSuffix(strings.ToLower(base), ".part") || !audioExtensions[strings.ToLower(filepath.Ext(base))] || time.Since(info.ModTime()) < fileStabilityWindow {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

// IsHighConfidence permits automated tag writing only for exact normalized
// title and artist agreement. Catalogue data is otherwise review-only.
func IsHighConfidence(track Track, candidate Candidate) bool {
	return normalized(track.Title) != "" && normalized(track.Artist) != "" &&
		normalized(track.Title) == normalized(candidate.Title) &&
		normalized(track.Artist) == normalized(candidate.Artist)
}

// ParseFilename derives a conservative artist, album and title hint when tags are absent.
// It supports the conventional Artist - Album - 01 - Title naming form.
func ParseFilename(path string) Track {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	parts := strings.Split(base, " - ")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	switch len(parts) {
	case 0:
		return Track{}
	case 1:
		return Track{Title: parts[0]}
	case 2:
		return Track{Artist: parts[0], Title: parts[1]}
	case 3:
		return Track{Artist: parts[0], Album: parts[1], Title: parts[2]}
	default:
		trackPart := parts[len(parts)-1]
		return Track{Artist: parts[0], Album: strings.Join(parts[1:len(parts)-2], " - "), Title: trackPart}
	}
}

func normalized(s string) string {
	var out []rune
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return string(out)
}
