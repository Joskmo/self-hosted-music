package auth

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type NavidromeUser struct {
	ID       string `json:"id"`
	UserName string `json:"userName"`
	Name     string `json:"name"`
	IsAdmin  bool   `json:"isAdmin"`
}

func GenerateInviteCode() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func navidromeURL() string {
	if u := os.Getenv("NAVIDROME_URL"); u != "" {
		return u
	}
	return "http://navidrome:4533"
}

func adminCreds() (string, string) {
	user := os.Getenv("NAVIDROME_ADMIN_USER")
	if user == "" {
		user = "admin"
	}
	pass := os.Getenv("NAVIDROME_ADMIN_PASSWORD")
	return user, pass
}

func GetNavidromeAdminToken() (string, error) {
	adminUser, adminPass := adminCreds()
	loginBody := map[string]string{
		"username": adminUser,
		"password": adminPass,
	}
	loginData, _ := json.Marshal(loginBody)

	req, err := http.NewRequest("POST", navidromeURL()+"/auth/login", bytes.NewReader(loginData))
	if err != nil {
		return "", fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("no token in login response: status %d", resp.StatusCode)
	}
	return result.Token, nil
}

func CheckNavidromeCredentials(username, password string) error {
	body := map[string]string{
		"username": username,
		"password": password,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", navidromeURL()+"/auth/login", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("navidrome request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid credentials")
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if result.Token == "" {
		return fmt.Errorf("invalid credentials")
	}

	return nil
}

func GetNavidromeUsers(token string) ([]NavidromeUser, error) {
	req, err := http.NewRequest("GET", navidromeURL()+"/api/user", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-ND-Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var users []NavidromeUser
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return users, nil
}

func CreateNavidromeUser(username, password, name string) (string, error) {
	token, err := GetNavidromeAdminToken()
	if err != nil {
		return "", fmt.Errorf("admin login: %w", err)
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

	req, err := http.NewRequest("POST", navidromeURL()+"/api/user", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ND-Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("navidrome request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("navidrome: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var created NavidromeUser
	if err := json.NewDecoder(resp.Body).Decode(&created); err == nil && created.ID != "" {
		return created.ID, nil
	}
	// Fallback: try to find user by username if create response doesn't include ID
	users, err := GetNavidromeUsers(token)
	if err != nil {
		return "", nil // not critical, will sync later
	}
	for _, u := range users {
		if u.UserName == username {
			return u.ID, nil
		}
	}
	return "", nil
}

func UpdateNavidromeUser(token, userID string, isAdmin bool) error {
	body := map[string]any{
		"isAdmin": isAdmin,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequest("PUT", navidromeURL()+"/api/user/"+userID, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ND-Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("navidrome: status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func DeleteNavidromeUser(token, userID string) error {
	req, err := http.NewRequest("DELETE", navidromeURL()+"/api/user/"+userID, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("X-ND-Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("navidrome: status %d, body: %s", resp.StatusCode, string(respBody))
	}
	return nil
}
