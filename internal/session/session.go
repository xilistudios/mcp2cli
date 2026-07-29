// Package session provides persistent MCP session management for mcp2cli.
// A background daemon holds one long-lived MCP connection and serves
// requests over a Unix domain socket. This package handles session
// lifecycle (start/stop/list) and client-side request routing.
package session

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/xilistudios/mcp2cli/internal/cache"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// SessionsDir returns the directory where session metadata, sockets,
// and log files are stored.
func SessionsDir() string {
	return filepath.Join(cache.CacheDir(), "sessions")
}

// metaPath returns the path to the session metadata JSON file.
func metaPath(name string) string {
	return filepath.Join(SessionsDir(), name+".json")
}

// sockPath returns the path to the Unix domain socket for a session.
func sockPath(name string) string {
	return filepath.Join(SessionsDir(), name+".sock")
}

// logPath returns the path to the daemon log file for a session.
func logPath(name string) string {
	return filepath.Join(SessionsDir(), name+".log")
}

// Meta holds the persisted metadata for a running session daemon.
type Meta struct {
	Pid       int     `json:"pid"`
	Source    string  `json:"source"`
	Transport string  `json:"transport"`
	CreatedAt float64 `json:"created_at"`
}

// Info extends Meta with runtime fields for listing.
type Info struct {
	Meta
	Name  string `json:"name"`
	Alive bool   `json:"alive"`
}

// IsAlive reports whether the given PID is currently running.
func IsAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// List returns all known sessions (both alive and dead).
// Returns nil, nil if the sessions directory does not exist.
func List() ([]Info, error) {
	dir := SessionsDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("globbing sessions: %w", err)
	}

	var sessions []Info
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var meta Meta
		if err := json.Unmarshal(raw, &meta); err != nil {
			continue
		}
		name := strings.TrimSuffix(filepath.Base(m), ".json")
		sessions = append(sessions, Info{
			Meta:  meta,
			Name:  name,
			Alive: IsAlive(meta.Pid),
		})
	}
	return sessions, nil
}

// Stop terminates a named session and cleans up its files.
func Stop(name string) error {
	mp := metaPath(name)
	raw, err := os.ReadFile(mp)
	if err == nil {
		var meta Meta
		if jsonErr := json.Unmarshal(raw, &meta); jsonErr == nil {
			if meta.Pid > 0 && IsAlive(meta.Pid) {
				syscall.Kill(meta.Pid, syscall.SIGTERM)
				// Poll up to ~1s for clean shutdown.
				for i := 0; i < 10; i++ {
					time.Sleep(100 * time.Millisecond)
					if !IsAlive(meta.Pid) {
						break
					}
				}
			}
		}
	}

	// Clean up files (ignore not-exist errors).
	os.Remove(mp)
	os.Remove(sockPath(name))
	os.Remove(logPath(name))
	return nil
}

// Start launches a new session daemon process.
func Start(name, source string, isStdio bool, authHeaders [][2]string, envVars map[string]string, transport string) error {
	if err := os.MkdirAll(SessionsDir(), 0755); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}

	mp := metaPath(name)

	// Check for an existing session.
	if raw, err := os.ReadFile(mp); err == nil {
		var meta Meta
		if jsonErr := json.Unmarshal(raw, &meta); jsonErr == nil {
			if meta.Pid > 0 && IsAlive(meta.Pid) {
				return fmt.Errorf("session '%s' is already running (PID %d)", name, meta.Pid)
			}
		}
		// Stale session — clean up.
		os.Remove(mp)
		os.Remove(sockPath(name))
	}

	// Build daemon config.
	cfg := DaemonConfig{
		Name:        name,
		Source:      source,
		IsStdio:     isStdio,
		AuthHeaders: authHeaders,
		EnvVars:     envVars,
		Transport:   transport,
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling daemon config: %w", err)
	}

	// Find the current executable.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	// Open log file.
	logFile, err := os.OpenFile(logPath(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()

	// Start the daemon process.
	cmd := newExecCommand(exe, "__session-daemon", string(cfgJSON))
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Watch for early exit.
	exitCh := make(chan int, 1)
	go func() {
		waitErr := cmd.Wait()
		code := 0
		if waitErr != nil {
			code = 1
			if exitErr, ok := waitErr.(exitCoder); ok {
				code = exitErr.ExitCode()
			}
		}
		exitCh <- code
	}()

	sp := sockPath(name)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sp); err == nil {
			fmt.Fprintf(util.Out, "Session '%s' started (PID %d)\n", name, cmd.Process.Pid)
			return nil
		}
		select {
		case code := <-exitCh:
			return fmt.Errorf("session daemon exited with code %d", code)
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Timeout — kill the process.
	cmd.Process.Kill()
	return fmt.Errorf("session daemon did not start in time")
}

// Request sends a JSON-RPC-style request to a named session daemon and
// returns the result.
func Request(name, method string, params map[string]any) (any, error) {
	sp := sockPath(name)
	if _, err := os.Stat(sp); os.IsNotExist(err) {
		return nil, fmt.Errorf("session '%s' not found", name)
	}

	conn, err := net.Dial("unix", sp)
	if err != nil {
		return nil, fmt.Errorf("connecting to session '%s': %w", name, err)
	}

	req := map[string]any{"id": 1, "method": method, "params": params}
	if req["params"] == nil {
		req["params"] = map[string]any{}
	}
	b, err := json.Marshal(req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("marshaling request: %w", err)
	}
	b = append(b, '\n')

	if _, err := conn.Write(b); err != nil {
		conn.Close()
		return nil, fmt.Errorf("writing request: %w", err)
	}
	if tc, ok := conn.(*net.UnixConn); ok {
		tc.CloseWrite()
	}

	// Read all data until EOF.
	var buf []byte
	tmp := make([]byte, 65536)
	for {
		n, readErr := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	conn.Close()

	if len(buf) == 0 {
		return nil, fmt.Errorf("empty response from session '%s'", name)
	}

	var resp map[string]any
	if err := json.Unmarshal(buf, &resp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if errVal, ok := resp["error"]; ok {
		return nil, fmt.Errorf("%v", errVal)
	}

	return resp["result"], nil
}
