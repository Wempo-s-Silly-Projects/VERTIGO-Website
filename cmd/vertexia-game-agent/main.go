// Copyright (C) 2026 ItsWempo
// SPDX-License-Identifier: AGPL-3.0-or-later

// vertexia-game-agent runs on a dedicated Linux game-server host. It exposes a
// small authenticated control API; game traffic itself remains direct UDP.
package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const readyMarker = "Start trigger dispatched"

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

type config struct {
	listen    string
	token     string
	bin       string
	workdir   string
	mapPath   string
	bind      string
	apiDomain string
	apiKey    string
	stateDir  string
	logDir    string
	portMin   int
	portMax   int
}

type serverRecord struct {
	ID      string    `json:"id"`
	GameID  int       `json:"game_id"`
	Port    int       `json:"port"`
	PID     int       `json:"pid"`
	Started time.Time `json:"started"`
}

type launchRequest struct {
	ID     string `json:"id"`
	GameID int    `json:"game_id"`
	Port   int    `json:"port"`
}

type serverStatus struct {
	ID      string `json:"id"`
	GameID  int    `json:"game_id"`
	Port    int    `json:"port"`
	PID     int    `json:"pid"`
	Running bool   `json:"running"`
	Ready   bool   `json:"ready"`
}

type agent struct {
	cfg config
	mu sync.Mutex
	records map[int]serverRecord
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" { return value }
	return fallback
}

func loadConfig() (config, error) {
	portMin, err := strconv.Atoi(env("GSM_PORT_MIN", "5100"))
	if err != nil { return config{}, fmt.Errorf("invalid GSM_PORT_MIN: %w", err) }
	portMax, err := strconv.Atoi(env("GSM_PORT_MAX", "5250"))
	if err != nil { return config{}, fmt.Errorf("invalid GSM_PORT_MAX: %w", err) }
	cfg := config{
		listen: env("GSM_AGENT_LISTEN", "127.0.0.1:8090"),
		token: env("GSM_AGENT_TOKEN", ""),
		bin: env("GSM_SERVER_BIN", "/opt/vertexia/bin/RaveEngineServer"),
		workdir: env("GSM_SERVER_WORKDIR", "/opt/vertexia"),
		mapPath: env("GSM_SERVER_MAP", "assets/maps/default.vrtx"),
		bind: env("GSM_BIND_HOST", "0.0.0.0"),
		apiDomain: env("VERTEXIA_API_DOMAIN", "https://vertexia.wempos-forum.com"),
		apiKey: env("GAMESERVER_API_KEY", ""),
		stateDir: env("GSM_AGENT_STATE_DIR", "/var/lib/vertexia-game-agent"),
		logDir: env("GSM_SERVER_LOGS_DIR", "/var/log/vertexia/gameservers"),
		portMin: portMin,
		portMax: portMax,
	}
	if len(cfg.token) < 32 { return config{}, errors.New("GSM_AGENT_TOKEN must contain at least 32 characters") }
	if cfg.apiKey == "" { return config{}, errors.New("GAMESERVER_API_KEY is required") }
	if portMin < 1024 || portMax < portMin || portMax > 65535 { return config{}, errors.New("invalid game-server UDP port range") }
	if !strings.HasPrefix(cfg.apiDomain, "https://") { return config{}, errors.New("VERTEXIA_API_DOMAIN must use HTTPS") }
	if _, err := os.Stat(cfg.bin); err != nil { return config{}, fmt.Errorf("game-server binary is unavailable: %w", err) }
	if err := os.MkdirAll(cfg.stateDir, 0700); err != nil { return config{}, err }
	if err := os.MkdirAll(cfg.logDir, 0750); err != nil { return config{}, err }
	return cfg, nil
}

func newAgent(cfg config) (*agent, error) {
	a := &agent{cfg: cfg, records: make(map[int]serverRecord)}
	entries, err := os.ReadDir(cfg.stateDir)
	if err != nil { return nil, err }
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") { continue }
		raw, err := os.ReadFile(filepath.Join(cfg.stateDir, entry.Name()))
		if err != nil { continue }
		var record serverRecord
		if json.Unmarshal(raw, &record) != nil || record.Port < cfg.portMin || record.Port > cfg.portMax || !safeID.MatchString(record.ID) { continue }
		if processMatches(record.PID, record.ID) { a.records[record.Port] = record } else { _ = os.Remove(filepath.Join(cfg.stateDir, entry.Name())) }
	}
	return a, nil
}

func (a *agent) recordPath(id string) string { return filepath.Join(a.cfg.stateDir, id+".json") }
func (a *agent) logPath(id string) string { return filepath.Join(a.cfg.logDir, "gs-"+id+".log") }

func processMatches(pid int, id string) bool {
	if pid <= 0 { return false }
	proc, err := os.FindProcess(pid)
	if err != nil || proc.Signal(syscall.Signal(0)) != nil { return false }
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil { return false }
	cmdline := strings.ReplaceAll(string(raw), "\x00", " ")
	return strings.Contains(cmdline, "--server-id "+id)
}

func (a *agent) running(record serverRecord) bool { return processMatches(record.PID, record.ID) }

func (a *agent) ready(id string) bool {
	raw, err := os.ReadFile(a.logPath(id))
	return err == nil && strings.Contains(string(raw), readyMarker)
}

func (a *agent) launch(w http.ResponseWriter, r *http.Request) {
	var input launchRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest); return
	}
	if !safeID.MatchString(input.ID) || input.GameID <= 0 || input.Port < a.cfg.portMin || input.Port > a.cfg.portMax {
		http.Error(w, "invalid server configuration", http.StatusBadRequest); return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if existing, ok := a.records[input.Port]; ok && a.running(existing) {
		if existing.ID == input.ID { writeJSON(w, serverStatus{ID: existing.ID, GameID: existing.GameID, Port: existing.Port, PID: existing.PID, Running: true, Ready: a.ready(existing.ID)}); return }
		http.Error(w, "port is already assigned to a running server", http.StatusConflict); return
	}
	if _, err := net.ListenPacket("udp", net.JoinHostPort("0.0.0.0", strconv.Itoa(input.Port))); err != nil {
		http.Error(w, "UDP port is already in use", http.StatusConflict); return
	} else { _ = err; /* the socket is released below before the child binds */ }
	// The probe above must be closed before the game process binds the port.
	probe, _ := net.ListenPacket("udp", net.JoinHostPort("0.0.0.0", strconv.Itoa(input.Port)))
	if probe != nil { _ = probe.Close() }
	logFile, err := os.OpenFile(a.logPath(input.ID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil { http.Error(w, "could not open server log", http.StatusInternalServerError); return }
	args := []string{"--port", strconv.Itoa(input.Port), "--map", a.cfg.mapPath, "--bind", a.cfg.bind, "--game-id", strconv.Itoa(input.GameID), "--server-id", input.ID}
	cmd := exec.Command(a.cfg.bin, args...)
	cmd.Dir = a.cfg.workdir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.Env = append(os.Environ(),
		"VERTEXIA_APP=server", "VERTIGO_APP=server",
		"VERTEXIA_GAME_ID="+strconv.Itoa(input.GameID), "VERTEXIA_SERVER_ID="+input.ID,
		"VERTEXIA_API_DOMAIN="+a.cfg.apiDomain, "VERTIGO_API_DOMAIN="+a.cfg.apiDomain,
		"GAMESERVER_API_KEY="+a.cfg.apiKey)
	if err := cmd.Start(); err != nil { _ = logFile.Close(); http.Error(w, "could not start game server", http.StatusInternalServerError); return }
	_ = logFile.Close()
	record := serverRecord{ID: input.ID, GameID: input.GameID, Port: input.Port, PID: cmd.Process.Pid, Started: time.Now().UTC()}
	raw, err := json.Marshal(record)
	if err == nil { err = os.WriteFile(a.recordPath(input.ID), raw, 0600) }
	if err != nil {
		_ = cmd.Process.Kill()
		http.Error(w, "could not persist server process record", http.StatusInternalServerError); return
	}
	a.records[input.Port] = record
	go func() { _ = cmd.Wait() }()
	writeJSON(w, serverStatus{ID: record.ID, GameID: record.GameID, Port: record.Port, PID: record.PID, Running: true, Ready: false})
}

func (a *agent) status(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/v1/servers/"))
	if err != nil || port < a.cfg.portMin || port > a.cfg.portMax { http.Error(w, "invalid port", http.StatusBadRequest); return }
	a.mu.Lock()
	defer a.mu.Unlock()
	record, ok := a.records[port]
	if !ok { http.NotFound(w, r); return }
	running := a.running(record)
	if !running {
		delete(a.records, port)
		_ = os.Remove(a.recordPath(record.ID))
		http.NotFound(w, r); return
	}
	writeJSON(w, serverStatus{ID: record.ID, GameID: record.GameID, Port: record.Port, PID: record.PID, Running: true, Ready: a.ready(record.ID)})
}

func (a *agent) stop(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/v1/servers/"))
	if err != nil || port < a.cfg.portMin || port > a.cfg.portMax { http.Error(w, "invalid port", http.StatusBadRequest); return }
	a.mu.Lock()
	record, ok := a.records[port]
	if !ok { a.mu.Unlock(); w.WriteHeader(http.StatusNoContent); return }
	delete(a.records, port)
	_ = os.Remove(a.recordPath(record.ID))
	a.mu.Unlock()
	proc, err := os.FindProcess(record.PID)
	if err == nil && a.running(record) {
		_ = proc.Signal(syscall.SIGTERM)
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) && a.running(record) { time.Sleep(100 * time.Millisecond) }
		if a.running(record) { _ = proc.Kill() }
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func (a *agent) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if len(provided) != len(a.cfg.token) || subtle.ConstantTimeCompare([]byte(provided), []byte(a.cfg.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized); return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	cfg, err := loadConfig()
	if err != nil { log.Fatal(err) }
	a, err := newAgent(cfg)
	if err != nil { log.Fatal(err) }
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	protected := http.NewServeMux()
	protected.HandleFunc("POST /v1/servers", a.launch)
	protected.HandleFunc("GET /v1/servers/{port}", a.status)
	protected.HandleFunc("DELETE /v1/servers/{port}", a.stop)
	mux.Handle("/v1/", a.authenticate(protected))
	server := &http.Server{Addr: cfg.listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("VERTEXIA game agent listening on %s with UDP ports %d-%d", cfg.listen, cfg.portMin, cfg.portMax)
	log.Fatal(server.ListenAndServe())
}
