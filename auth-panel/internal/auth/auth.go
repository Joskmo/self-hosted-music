package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

func GenerateInviteCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func CheckNavidromeCredentials(username, password string) error {
	navURL := os.Getenv("NAVIDROME_URL")
	if navURL == "" {
		navURL = "http://navidrome:4533"
	}

	reqURL := fmt.Sprintf("%s/rest/ping?u=%s&p=%s&v=1.16.0&c=auth-panel&f=json",
		navURL,
		url.QueryEscape(username),
		url.QueryEscape(password),
	)

	resp, err := http.Get(reqURL)
	if err != nil {
		return fmt.Errorf("navidrome request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		SubsonicResponse struct {
			Status string `json:"status"`
		} `json:"subsonic-response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if result.SubsonicResponse.Status != "ok" {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

func CreateNavidromeUser(username, password, name string) error {
	navURL := os.Getenv("NAVIDROME_URL")
	if navURL == "" {
		navURL = "http://navidrome:4533"
	}
	adminUser := os.Getenv("NAVIDROME_ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPass := os.Getenv("NAVIDROME_ADMIN_PASSWORD")

	loginBody := map[string]string{
		"username": adminUser,
		"password": adminPass,
	}
	loginData, _ := json.Marshal(loginBody)

	loginReq, err := http.NewRequest("POST", navURL+"/auth/login", bytes.NewReader(loginData))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	loginReq.Header.Set("Content-Type", "application/json")

	loginResp, err := http.DefaultClient.Do(loginReq)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer loginResp.Body.Close()

	var loginResult struct {
		Token   string `json:"token"`
		ID      string `json:"id"`
		IsAdmin bool   `json:"isAdmin"`
	}
	if err := json.NewDecoder(loginResp.Body).Decode(&loginResult); err != nil {
		return fmt.Errorf("decode login response: %w", err)
	}
	if loginResult.Token == "" && loginResult.ID == "" {
		return fmt.Errorf("no token in login response: status %d", loginResp.StatusCode)
	}
	authToken := loginResult.Token
	if authToken == "" {
		authToken = loginResult.ID
	}

	body := map[string]any{
		"userName": username,
		"password": password,
	}
	if name != "" {
		body["name"] = name
	} else {
		body["name"] = username
	}

	data, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", navURL+"/api/user", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ND-Authorization", "Bearer "+loginResult.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("navidrome request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("navidrome: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
