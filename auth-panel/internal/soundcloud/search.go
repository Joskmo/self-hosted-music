package soundcloud

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const maxResponseBytes = 4 << 20

type Mode string

const (
	Everything  Mode = "everything"
	Tracks      Mode = "tracks"
	Albums      Mode = "albums"
	Playlists   Mode = "playlists"
	Collections Mode = "collections"
)

type Track struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	URL    string `json:"url"`
	Type   Mode   `json:"type"`
}

type Client struct {
	baseURL    string
	apiBaseURL string
	httpClient *http.Client
	clientID   string
	clientMu   sync.Mutex
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	apiBaseURL := "https://api-v2.soundcloud.com"
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Hostname() != "soundcloud.com" && parsed.Hostname() != "www.soundcloud.com" {
		apiBaseURL = strings.TrimRight(baseURL, "/")
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiBaseURL: apiBaseURL, httpClient: &clientCopy}
}

func (m Mode) Valid() bool {
	return m == Everything || m == Tracks || m == Albums || m == Playlists
}

func ParseMode(raw string) (Mode, error) {
	if raw == "" {
		return Everything, nil
	}
	mode := Mode(raw)
	if !mode.Valid() {
		return "", errors.New("неподдерживаемый режим поиска")
	}
	return mode, nil
}

func (m Mode) endpointPath() string {
	switch m {
	case Everything:
		return "/search"
	case Albums:
		return "/search/albums"
	case Playlists:
		return "/search/playlists"
	default:
		return "/search/sounds"
	}
}

func (c *Client) Search(ctx context.Context, query string, requestedMode ...Mode) ([]Track, error) {
	mode := Tracks
	if len(requestedMode) > 0 {
		mode = requestedMode[0]
	}
	if !mode.Valid() {
		return nil, errors.New("неподдерживаемый режим поиска")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("введите название или исполнителя")
	}
	if len([]rune(query)) > 160 {
		return nil, errors.New("слишком длинный запрос")
	}

	if mode == Everything {
		return c.searchAPI(ctx, query, Tracks)
	}
	if mode == Tracks || mode == Albums || mode == Playlists {
		return c.searchAPI(ctx, query, mode)
	}

	endpoint, err := url.Parse(c.baseURL + mode.endpointPath())
	if err != nil {
		return nil, fmt.Errorf("soundcloud endpoint: %w", err)
	}
	endpoint.RawQuery = url.Values{"q": {query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("soundcloud request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("поиск SoundCloud недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("поиск SoundCloud вернул HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("чтение ответа SoundCloud: %w", err)
	}
	if len(body) > maxResponseBytes {
		return nil, errors.New("слишком большой ответ SoundCloud")
	}
	return parseResults(string(body), mode), nil
}

var (
	listItemRe = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	anchorRe   = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	tagRe      = regexp.MustCompile(`(?is)<[^>]+>`)
)

func parseTracks(page string) []Track {
	return parseResults(page, Tracks)
}

func parseResults(page string, mode Mode) []Track {
	results := make([]Track, 0, 10)
	seen := make(map[string]struct{})
	for _, item := range listItemRe.FindAllStringSubmatch(page, -1) {
		anchors := anchorRe.FindAllStringSubmatch(item[1], -1)
		var track Track
		for _, anchor := range anchors {
			href := html.UnescapeString(anchor[1])
			label := cleanText(anchor[2])
			if track.URL == "" && isResultPath(href, mode) {
				track.URL = "https://soundcloud.com" + href
				track.Title = label
				track.Type = resultType(mode, href)
				continue
			}
			if track.URL != "" && track.Artist == "" && isProfilePath(href) {
				track.Artist = label
			}
		}
		if track.URL != "" && track.Artist == "" {
			track.Artist = artistFromResultURL(track.URL)
		}
		if track.URL == "" || track.Title == "" || track.Artist == "" {
			continue
		}
		if _, duplicate := seen[track.URL]; duplicate {
			continue
		}
		seen[track.URL] = struct{}{}
		results = append(results, track)
		if len(results) == 20 {
			break
		}
	}
	return results
}

func isResultPath(href string, mode Mode) bool {
	if mode == Albums || mode == Playlists {
		return isCollectionPath(href)
	}
	if mode == Everything {
		return isTrackPath(href) || isCollectionPath(href)
	}
	return isTrackPath(href)
}

func resultType(mode Mode, href string) Mode {
	if mode == Everything && isCollectionPath(href) {
		return Collections
	}
	return mode
}

func cleanText(value string) string {
	return strings.TrimSpace(html.UnescapeString(strings.Join(strings.Fields(tagRe.ReplaceAllString(value, " ")), " ")))
}

func artistFromResultURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" {
		return ""
	}
	return strings.ReplaceAll(parts[0], "-", " ")
}

func isTrackPath(href string) bool {
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return false
	}
	parts := strings.Split(strings.Trim(href, "/"), "/")
	return len(parts) == 2 && parts[0] != "search" && parts[0] != "discover"
}

func isCollectionPath(href string) bool {
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return false
	}
	parts := strings.Split(strings.Trim(href, "/"), "/")
	return len(parts) == 3 && parts[0] != "" && parts[1] == "sets" && parts[2] != ""
}

func isProfilePath(href string) bool {
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return false
	}
	parts := strings.Split(strings.Trim(href, "/"), "/")
	return len(parts) == 1 && parts[0] != ""
}
