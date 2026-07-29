package metube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "http://metube:8081"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: &clientCopy}
}

func (c *Client) AddAudio(ctx context.Context, sourceURL string) error {
	if !isSoundCloudURL(sourceURL) {
		return errors.New("можно добавить только ссылку SoundCloud из результатов поиска")
	}
	payload := map[string]any{
		"url": sourceURL, "download_type": "audio", "codec": "auto", "format": "mp3", "quality": "best",
		"folder": "", "custom_name_prefix": "", "playlist_item_limit": 0, "auto_start": true,
		"split_by_chapters": false, "chapter_template": "", "subtitle_language": "", "subtitle_mode": "",
		"ytdl_options_presets": []string{}, "ytdl_options_overrides": "",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("кодирование запроса MeTube: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/add", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("создание запроса MeTube: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("MeTube недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("MeTube вернул HTTP %d", resp.StatusCode)
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("ответ MeTube: %w", err)
	}
	if result.Status != "ok" {
		return errors.New("MeTube не принял задачу")
	}
	return nil
}

func isSoundCloudURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host != "soundcloud.com" && host != "www.soundcloud.com" {
		return false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}
