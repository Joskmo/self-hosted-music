package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const musicBrainzURL = "https://musicbrainz.org/ws/2/recording"

var musicBrainzLimiter struct {
	sync.Mutex
	last time.Time
}

// Processor is the single asynchronous pipeline for uploads and MeTube files.
type Processor struct {
	db       *sql.DB
	musicDir string
	client   *http.Client
	seen     map[string]fileObservation
}

type fileObservation struct {
	size     int64
	modified time.Time
}

func unchangedSincePreviousScan(seen map[string]fileObservation, path string, current fileObservation) bool {
	previous, ok := seen[path]
	seen[path] = current
	return ok && previous == current
}

func NewProcessor(database *sql.DB, musicDir string) *Processor {
	return &Processor{db: database, musicDir: musicDir, client: &http.Client{Timeout: 10 * time.Second}, seen: make(map[string]fileObservation)}
}

// Scan discovers completed files. It intentionally does not alter audio tags;
// all catalogue results enter the review queue.
func (p *Processor) Scan(ctx context.Context) error {
	files, err := DiscoverAudioFiles(p.musicDir)
	if err != nil {
		return err
	}
	for _, path := range files {
		info, err := os.Stat(filepath.Join(p.musicDir, filepath.FromSlash(path)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !unchangedSincePreviousScan(p.seen, path, fileObservation{size: info.Size(), modified: info.ModTime()}) {
			continue
		}
		var status string
		err = p.db.QueryRowContext(ctx, "SELECT status FROM music_metadata WHERE file_path = $1", path).Scan(&status)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil && status != "unavailable" {
			continue
		}
		if err := p.process(ctx, path); err != nil {
			return err
		}
	}
	return nil
}

// Reprocess requests a fresh catalogue lookup without modifying the audio file.
func (p *Processor) Reprocess(ctx context.Context, path string) error {
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return fmt.Errorf("invalid file path")
	}
	return p.process(ctx, path)
}

func (p *Processor) process(ctx context.Context, path string) error {
	track := extractTrack(filepath.Join(p.musicDir, filepath.FromSlash(path)))
	if track.Title == "" {
		track = ParseFilename(path)
	}
	candidate, status, lookupErr := p.lookup(ctx, track)
	confidence := 0.0
	if status == "matched" && IsHighConfidence(track, candidate) {
		confidence = 1.0
	}
	if status == "matched" {
		status = "review"
	}
	if lookupErr != nil {
		status = "unavailable"
	}
	_, err := p.db.ExecContext(ctx, `INSERT INTO music_metadata
		(file_path, source, original_title, original_artist, original_album, suggested_title, suggested_artist, suggested_album, musicbrainz_id, confidence, status, last_error, updated_at)
		VALUES ($1, 'scanner', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (file_path) DO UPDATE SET original_title=EXCLUDED.original_title, original_artist=EXCLUDED.original_artist, original_album=EXCLUDED.original_album, suggested_title=EXCLUDED.suggested_title, suggested_artist=EXCLUDED.suggested_artist, suggested_album=EXCLUDED.suggested_album, musicbrainz_id=EXCLUDED.musicbrainz_id, confidence=EXCLUDED.confidence, status=EXCLUDED.status, last_error=EXCLUDED.last_error, updated_at=NOW()`,
		path, track.Title, track.Artist, track.Album, candidate.Title, candidate.Artist, candidate.Album, candidate.ID, confidence, status, errorText(lookupErr))
	return err
}

func (p *Processor) lookup(ctx context.Context, track Track) (candidate Candidate, status string, err error) {
	if strings.TrimSpace(track.Title) == "" {
		return candidate, "no_match", nil
	}
	q := url.Values{"query": {"recording:" + quoteQuery(track.Title)}}
	if track.Artist != "" {
		q.Set("query", "recording:"+quoteQuery(track.Title)+" AND artist:"+quoteQuery(track.Artist))
	}
	if err := waitForMusicBrainz(ctx); err != nil {
		return candidate, "unavailable", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, musicBrainzURL+"?"+q.Encode()+"&fmt=json&limit=1", nil)
	if err != nil {
		return candidate, "unavailable", err
	}
	req.Header.Set("User-Agent", "self-hosted-music/metadata-normalizer (admin@localhost)")
	resp, err := p.client.Do(req)
	if err != nil {
		return candidate, "unavailable", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return candidate, "unavailable", fmt.Errorf("MusicBrainz returned %s", resp.Status)
	}
	var data struct {
		Recordings []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Releases []struct {
				Title string `json:"title"`
			} `json:"releases"`
		} `json:"recordings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return candidate, "unavailable", err
	}
	if len(data.Recordings) == 0 {
		return candidate, "no_match", nil
	}
	r := data.Recordings[0]
	candidate.ID, candidate.Title = r.ID, r.Title
	if len(r.ArtistCredit) > 0 {
		candidate.Artist = r.ArtistCredit[0].Name
	}
	if len(r.Releases) > 0 {
		candidate.Album = r.Releases[0].Title
	}
	return candidate, "matched", nil
}

func waitForMusicBrainz(ctx context.Context) error {
	musicBrainzLimiter.Lock()
	defer musicBrainzLimiter.Unlock()
	if wait := time.Until(musicBrainzLimiter.last.Add(time.Second)); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	musicBrainzLimiter.last = time.Now()
	return nil
}

func quoteQuery(s string) string { return `"` + strings.ReplaceAll(s, `"`, "") + `"` }
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func extractTrack(path string) Track {
	filenameTrack := ParseFilename(path)
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format_tags=title,artist,album", "-of", "json", path).Output()
	if err != nil {
		return filenameTrack
	}
	var data struct {
		Format struct {
			Tags map[string]string `json:"tags"`
		} `json:"format"`
	}
	if json.Unmarshal(out, &data) != nil {
		return filenameTrack
	}
	t := Track{Title: data.Format.Tags["title"], Artist: data.Format.Tags["artist"], Album: data.Format.Tags["album"]}
	return mergeTrackHints(t, filenameTrack)
}

func mergeTrackHints(tags, filename Track) Track {
	if tags.Title == "" {
		tags.Title = filename.Title
	}
	if tags.Artist == "" {
		tags.Artist = filename.Artist
	}
	if tags.Album == "" {
		tags.Album = filename.Album
	}
	return tags
}
