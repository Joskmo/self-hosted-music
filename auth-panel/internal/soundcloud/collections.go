package soundcloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var (
	scriptSrcRe = regexp.MustCompile(`(?is)<script[^>]+src=["']([^"']+)["']`)
	clientIDRe  = regexp.MustCompile(`client_id\s*:\s*"([0-9a-zA-Z]{32})"`)
)

type collectionResponse struct {
	Collection []struct {
		Title        string `json:"title"`
		PermalinkURL string `json:"permalink_url"`
		User         struct {
			Username string `json:"username"`
		} `json:"user"`
	} `json:"collection"`
}

func (c *Client) searchAPI(ctx context.Context, query string, mode Mode) ([]Track, error) {
	clientID, err := c.publicClientID(ctx)
	if err != nil {
		return nil, err
	}

	endpoint, err := url.Parse(c.apiBaseURL + "/search/" + string(mode))
	if err != nil {
		return nil, fmt.Errorf("SoundCloud collection endpoint: %w", err)
	}
	endpoint.RawQuery = url.Values{
		"q":         {query},
		"limit":     {"20"},
		"client_id": {clientID},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("SoundCloud collection request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")

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

	var payload collectionResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("разбор выдачи SoundCloud: %w", err)
	}
	results := make([]Track, 0, len(payload.Collection))
	seen := make(map[string]struct{})
	for _, item := range payload.Collection {
		validURL := isCanonicalCollectionURL(item.PermalinkURL)
		if mode == Tracks {
			validURL = isCanonicalTrackURL(item.PermalinkURL)
		}
		if item.Title == "" || item.User.Username == "" || !validURL {
			continue
		}
		if _, exists := seen[item.PermalinkURL]; exists {
			continue
		}
		seen[item.PermalinkURL] = struct{}{}
		results = append(results, Track{Title: item.Title, Artist: item.User.Username, URL: item.PermalinkURL, Type: mode})
	}
	return results, nil
}

func (c *Client) publicClientID(ctx context.Context) (string, error) {
	c.clientMu.Lock()
	defer c.clientMu.Unlock()
	if c.clientID != "" {
		return c.clientID, nil
	}

	pageURL, err := url.Parse(c.baseURL + "/")
	if err != nil {
		return "", fmt.Errorf("SoundCloud homepage: %w", err)
	}
	page, err := c.getPage(ctx, pageURL.String())
	if err != nil {
		return "", err
	}
	matches := scriptSrcRe.FindAllStringSubmatch(page, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		scriptURL, err := pageURL.Parse(strings.TrimSpace(matches[i][1]))
		if err != nil || (scriptURL.Host != pageURL.Host && scriptURL.Host != "a-v2.sndcdn.com") {
			continue
		}
		script, err := c.getPage(ctx, scriptURL.String())
		if err != nil {
			continue
		}
		if id := clientIDRe.FindStringSubmatch(script); len(id) == 2 {
			c.clientID = id[1]
			return c.clientID, nil
		}
	}
	return "", errors.New("не удалось получить публичную конфигурацию SoundCloud")
}

func (c *Client) getPage(ctx context.Context, rawURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SoundCloud вернул HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxResponseBytes {
		return "", errors.New("слишком большой ответ SoundCloud")
	}
	return string(body), nil
}

func isCanonicalTrackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || (u.Host != "soundcloud.com" && u.Host != "www.soundcloud.com") {
		return false
	}
	return isTrackPath(u.Path)
}

func isCanonicalCollectionURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || (u.Host != "soundcloud.com" && u.Host != "www.soundcloud.com") {
		return false
	}
	return isCollectionPath(u.Path)
}
