// Copyright (C) 2026 ItsWempo
// SPDX-License-Identifier: AGPL-3.0-or-later

package gsm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPLauncher struct {
	baseURL string
	token string
	client *http.Client
}

type agentLaunchRequest struct {
	ID string `json:"id"`
	GameID int `json:"game_id"`
	Port int `json:"port"`
}

type agentServerStatus struct {
	ID string `json:"id"`
	GameID int `json:"game_id"`
	Port int `json:"port"`
	PID int `json:"pid"`
	Running bool `json:"running"`
	Ready bool `json:"ready"`
}

func NewHTTPLauncher(baseURL, token string) (*HTTPLauncher, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("GSM_AGENT_URL must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "https" && parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" {
		return nil, fmt.Errorf("GSM_AGENT_URL must use HTTPS outside localhost")
	}
	if len(token) < 32 {
		return nil, fmt.Errorf("GSM_AGENT_TOKEN must contain at least 32 characters")
	}
	return &HTTPLauncher{baseURL: baseURL, token: token, client: &http.Client{Timeout: 5 * time.Second}}, nil
}

func (l *HTTPLauncher) request(method, path string, body any) (*http.Response, error) {
	var payload *bytes.Reader
	if body == nil { payload = bytes.NewReader(nil) } else {
		raw, err := json.Marshal(body)
		if err != nil { return nil, err }
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, l.baseURL+path, payload)
	if err != nil { return nil, err }
	req.Header.Set("Authorization", "Bearer "+l.token)
	if body != nil { req.Header.Set("Content-Type", "application/json") }
	return l.client.Do(req)
}

func (l *HTTPLauncher) Launch(id string, gameID int, port int) (int, error) {
	resp, err := l.request(http.MethodPost, "/v1/servers", agentLaunchRequest{ID:id, GameID:gameID, Port:port})
	if err != nil { return 0, fmt.Errorf("game agent launch request failed: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("game agent launch returned HTTP %d", resp.StatusCode)
	}
	var status agentServerStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil { return 0, err }
	if status.PID <= 0 || status.Port != port || status.ID != id { return 0, fmt.Errorf("game agent returned an invalid process record") }
	return status.PID, nil
}

func (l *HTTPLauncher) WaitHealthy(id string, port int, pid int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := l.request(http.MethodGet, "/v1/servers/"+strconv.Itoa(port), nil)
		if err == nil {
			var status agentServerStatus
			decodeErr := json.NewDecoder(resp.Body).Decode(&status)
			_ = resp.Body.Close()
			if decodeErr == nil && status.Running && status.Ready && status.ID == id && status.PID == pid { return nil }
			if decodeErr == nil && (!status.Running || status.ID != id || status.PID != pid) { return fmt.Errorf("game agent reports server %s is not running", id) }
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("game agent did not report server %s healthy before timeout", id)
}

func (l *HTTPLauncher) Alive(pid int, port int) bool {
	resp, err := l.request(http.MethodGet, "/v1/servers/"+strconv.Itoa(port), nil)
	if err != nil { return false }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK { return false }
	var status agentServerStatus
	if json.NewDecoder(resp.Body).Decode(&status) != nil { return false }
	return status.Running && status.PID == pid && status.Port == port
}

func (l *HTTPLauncher) Kill(pid int, port int) error {
	resp, err := l.request(http.MethodDelete, "/v1/servers/"+strconv.Itoa(port), nil)
	if err != nil { return err }
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("game agent stop returned HTTP %d", resp.StatusCode)
	}
	return nil
}
