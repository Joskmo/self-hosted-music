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
	"time"
)

const maxResponseBytes = 4 << 20

type Track struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
	URL    string `json:"url"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: &clientCopy}
}

func (c *Client) Search(ctx context.Context, query string) ([]Track, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("введите название или исполнителя")
	}
	if len([]rune(query)) > 160 {
		return nil, errors.New("слишком длинный запрос")
	}

	endpoint, err := url.Parse(c.baseURL + "/search/sounds")
	if err != nil {
		return nil, fmt.Errorf("soundcloud endpoint: %w", err)
	}
	endpoint.RawQuery = url.Values{"q": {query}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("soundcloud request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PersonalMusicLibrary/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

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
	return parseTracks(string(body)), nil
}

var (
	listItemRe = regexp.MustCompile(`(?is)<li\b[^>]*>(.*?)</li>`)
	anchorRe   = regexp.MustCompile(`(?is)<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	tagRe      = regexp.MustCompile(`(?is)<[^>]+>`)
)

func parseTracks(page string) []Track {
	results := make([]Track, 0, 10)
	seen := make(map[string]struct{})
	for _, item := range listItemRe.FindAllStringSubmatch(page, -1) {
		anchors := anchorRe.FindAllStringSubmatch(item[1], -1)
		var track Track
		for _, anchor := range anchors {
			href := html.UnescapeString(anchor[1])
			label := cleanText(anchor[2])
			if track.URL == "" && isTrackPath(href) {
				track.URL = "https://soundcloud.com" + href
				track.Title = label
				continue
			}
			if track.URL != "" && track.Artist == "" && isProfilePath(href) {
				track.Artist = label
			}
		}
		if track.URL != "" && track.Artist == "" {
			track.Artist = artistFromTrackURL(track.URL)
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

func cleanText(value string) string {
	return strings.TrimSpace(html.UnescapeString(strings.Join(strings.Fields(tagRe.ReplaceAllString(value, " ")), " ")))
}

func artistFromTrackURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 {
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

func isProfilePath(href string) bool {
	if !strings.HasPrefix(href, "/") || strings.HasPrefix(href, "//") {
		return false
	}
	parts := strings.Split(strings.Trim(href, "/"), "/")
	return len(parts) == 1 && parts[0] != ""
}
