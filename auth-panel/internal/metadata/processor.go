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
	"syscall"
	"time"
)

var musicBrainzURL = "https://musicbrainz.org/ws/2/recording"

// metadataUpsertSQL keeps the initial extracted tags immutable as an audit
// snapshot. Subsequent scans refresh only the catalogue result and its status.
const metadataUpsertSQL = `INSERT INTO music_metadata
	(file_path, source, original_title, original_artist, original_album, suggested_title, suggested_artist, suggested_album, musicbrainz_id, confidence, status, last_error, updated_at)
	VALUES ($1, 'scanner', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
	ON CONFLICT (file_path) DO UPDATE SET suggested_title=EXCLUDED.suggested_title, suggested_artist=EXCLUDED.suggested_artist, suggested_album=EXCLUDED.suggested_album, musicbrainz_id=EXCLUDED.musicbrainz_id, confidence=EXCLUDED.confidence, status=EXCLUDED.status, last_error=EXCLUDED.last_error, updated_at=NOW()`

var musicBrainzLimiter struct {
	sync.Mutex
	last time.Time
}

// ScanStatus is the current and most recently completed library scan state.
type ScanStatus struct {
	Running        bool      `json:"running"`
	StartedAt      time.Time `json:"started_at"`
	LastFinishedAt time.Time `json:"last_finished_at"`
	LastError      string    `json:"last_error"`
}

// Processor is the single asynchronous pipeline for uploads and MeTube files.
type Processor struct {
	db       *sql.DB
	musicDir string
	client   *http.Client
	seen     map[string]fileObservation
	scanMu   sync.Mutex
	statusMu sync.Mutex
	status   ScanStatus
}

type fileObservation struct {
	size     int64
	modified time.Time
}

var runFFmpeg = func(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

var chownFile = os.Chown

func ownershipOf(info os.FileInfo) (int, int, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("file ownership is unavailable")
	}
	return int(stat.Uid), int(stat.Gid), nil
}

// ApplyTags explicitly writes reviewed metadata using a temporary file in the
// same directory. The original is only replaced after ffmpeg succeeds.
func ApplyTags(ctx context.Context, path string, track Track) error {
	if strings.TrimSpace(track.Title) == "" || strings.TrimSpace(track.Artist) == "" {
		return fmt.Errorf("title and artist are required")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular audio file")
	}
	uid, gid, err := ownershipOf(info)
	if err != nil {
		return err
	}
	ext := filepath.Ext(path)
	temp, err := os.CreateTemp(filepath.Dir(path), ".metadata-*"+ext)
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		os.Remove(tempPath)
		return err
	}
	defer os.Remove(tempPath)
	if err := runFFmpeg(ctx, "ffmpeg", "-nostdin", "-v", "error", "-y", "-i", path, "-map", "0", "-c", "copy", "-metadata", "title="+track.Title, "-metadata", "artist="+track.Artist, "-metadata", "album="+track.Album, tempPath); err != nil {
		return fmt.Errorf("write audio tags: %w", err)
	}
	if err := os.Chmod(tempPath, info.Mode().Perm()); err != nil {
		return err
	}
	if err := chownFile(tempPath, uid, gid); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func unchangedSincePreviousScan(seen map[string]fileObservation, path string, current fileObservation) bool {
	previous, ok := seen[path]
	seen[path] = current
	return ok && previous == current
}

func NewProcessor(database *sql.DB, musicDir string) *Processor {
	return &Processor{db: database, musicDir: musicDir, client: musicBrainzHTTPClient(os.Getenv("MUSICBRAINZ_PROXY_URL")), seen: make(map[string]fileObservation)}
}

// musicBrainzHTTPClient keeps the proxy scope limited to public metadata
// lookups rather than changing networking for Navidrome, PostgreSQL or MeTube.
func musicBrainzHTTPClient(proxyRaw string) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy, err := url.Parse(proxyRaw); err == nil && proxy.Scheme != "" && proxy.Host != "" {
		transport.Proxy = http.ProxyURL(proxy)
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}
}

// StartScan launches a scan only when another scan is not already running.
// It returns false when an existing scan owns the library scanner.
func (p *Processor) StartScan() bool {
	p.statusMu.Lock()
	if p.status.Running {
		p.statusMu.Unlock()
		return false
	}
	p.status.Running = true
	p.status.StartedAt = time.Now().UTC()
	p.status.LastError = ""
	p.statusMu.Unlock()

	go func() {
		err := p.Scan(context.Background())
		p.statusMu.Lock()
		p.status.Running = false
		p.status.LastFinishedAt = time.Now().UTC()
		p.status.LastError = errorText(err)
		p.statusMu.Unlock()
	}()
	return true
}

// Status returns a copy safe for use by the administration API.
func (p *Processor) Status() ScanStatus {
	p.statusMu.Lock()
	defer p.statusMu.Unlock()
	return p.status
}

// Scan discovers completed files. It intentionally does not alter audio tags;
// all catalogue results enter the review queue.
func (p *Processor) Scan(ctx context.Context) error {
	p.scanMu.Lock()
	defer p.scanMu.Unlock()

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
	file, err := p.openLibraryFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return p.processTrack(ctx, path, extractTrackFromFile(file, path))
}

// Apply writes explicitly reviewed tags to a known library file.
func (p *Processor) Apply(ctx context.Context, path string, track Track) error {
	file, err := p.libraryFile(path)
	if err != nil {
		return err
	}
	return ApplyTags(ctx, file, track)
}

func (p *Processor) libraryFile(path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid file path")
	}
	root, err := filepath.Abs(p.musicDir)
	if err != nil {
		return "", err
	}
	file := filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
	if file != root && !strings.HasPrefix(file, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid file path")
	}
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		info, err := os.Lstat(dir)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("invalid music directory")
		}
		if dir == root {
			break
		}
	}
	info, err := os.Lstat(file)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular audio file")
	}
	return file, nil
}

// openLibraryFile resolves every path component through an already-open
// directory descriptor. O_NOFOLLOW on each open prevents a directory swap
// from redirecting reprocessing outside the music library.
func (p *Processor) openLibraryFile(path string) (*os.File, error) {
	if filepath.IsAbs(path) {
		return nil, fmt.Errorf("invalid file path")
	}
	root, err := filepath.Abs(p.musicDir)
	if err != nil {
		return nil, err
	}
	rel := filepath.Clean(filepath.FromSlash(path))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, fmt.Errorf("invalid file path")
	}
	fd, err := syscall.Open(root, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, part := range parts {
		flags := syscall.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC
		if i < len(parts)-1 {
			flags |= syscall.O_DIRECTORY
		} else {
			// A FIFO would otherwise block here before fstat can reject it.
			flags |= syscall.O_NONBLOCK
		}
		next, err := syscall.Openat(fd, part, flags, 0)
		syscall.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		syscall.Close(fd)
		return nil, fmt.Errorf("not a regular audio file")
	}
	return os.NewFile(uintptr(fd), filepath.Join(root, rel)), nil
}

func (p *Processor) process(ctx context.Context, path string) error {
	file, err := p.openLibraryFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return p.processTrack(ctx, path, extractTrackFromFile(file, path))
}

func (p *Processor) processTrack(ctx context.Context, path string, track Track) error {
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
	_, err := p.db.ExecContext(ctx, metadataUpsertSQL,
		path, track.Title, track.Artist, track.Album, candidate.Title, candidate.Artist, candidate.Album, candidate.ID, confidence, status, errorText(lookupErr))
	return err
}

func (p *Processor) lookup(ctx context.Context, track Track) (candidate Candidate, status string, err error) {
	if strings.TrimSpace(track.Title) == "" {
		return candidate, "no_match", nil
	}
	lookupTitle := normalizeLookupTitleForArtist(track.Title, track.Artist)
	q := url.Values{"query": {"recording:" + quoteQuery(lookupTitle)}}
	if track.Artist != "" {
		q.Set("query", "recording:"+quoteQuery(lookupTitle)+" AND artist:"+quoteQuery(track.Artist))
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
				Name       string `json:"name"`
				JoinPhrase string `json:"joinphrase"`
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
	for _, credit := range r.ArtistCredit {
		candidate.Artist += credit.Name + credit.JoinPhrase
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

func quoteQuery(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func extractTrack(path string) Track {
	filenameTrack := ParseFilename(path)
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format_tags=title,artist,album", "-of", "json", path).Output()
	return trackFromFFprobe(out, err, filenameTrack)
}

func extractTrackFromFile(file *os.File, path string) Track {
	filenameTrack := ParseFilename(path)
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format_tags=title,artist,album", "-of", "json", "pipe:3")
	cmd.ExtraFiles = []*os.File{file}
	out, err := cmd.Output()
	return trackFromFFprobe(out, err, filenameTrack)
}

func trackFromFFprobe(out []byte, err error, filenameTrack Track) Track {
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
