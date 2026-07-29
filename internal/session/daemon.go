package session

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/xilistudios/mcp2cli/internal/mcp"
)

// DaemonConfig holds the configuration passed to the session daemon
// process via command-line argument.
type DaemonConfig struct {
	Name        string            `json:"name"`
	Source      string            `json:"source"`
	IsStdio     bool              `json:"is_stdio"`
	AuthHeaders [][2]string       `json:"auth_headers"`
	EnvVars     map[string]string `json:"env_vars"`
	Transport   string            `json:"transport"`
}

// MCPOps is the subset of *mcp.Client the daemon uses. This interface
// allows unit testing with a fake implementation.
type MCPOps interface {
	ListTools(ctx context.Context) ([]map[string]any, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (*mcp.CallResult, error)
	ListResources(ctx context.Context) ([]map[string]any, error)
	ReadResource(ctx context.Context, uri string) (string, error)
	ListResourceTemplates(ctx context.Context) ([]map[string]any, error)
	ListPrompts(ctx context.Context) ([]map[string]any, error)
	GetPrompt(ctx context.Context, name string, arguments map[string]any) (map[string]any, error)
}

// HandleRequest dispatches one method call to the appropriate MCPOps
// method. This is the core routing logic, unit testable with a fake
// MCPOps implementation.
func HandleRequest(ctx context.Context, ops MCPOps, method string, params map[string]any) (any, error) {
	switch method {
	case "list_tools":
		return ops.ListTools(ctx)
	case "call_tool":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		r, err := ops.CallTool(ctx, name, args)
		if err != nil {
			return nil, err
		}
		return r.Text(), nil
	case "list_resources":
		return ops.ListResources(ctx)
	case "read_resource":
		uri, _ := params["uri"].(string)
		return ops.ReadResource(ctx, uri)
	case "list_resource_templates":
		return ops.ListResourceTemplates(ctx)
	case "list_prompts":
		return ops.ListPrompts(ctx)
	case "get_prompt":
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		return ops.GetPrompt(ctx, name, args)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// ServeConn handles a single connection: reads one line request,
// dispatches via HandleRequest, writes one line response, and closes.
func ServeConn(ctx context.Context, ops MCPOps, conn net.Conn) {
	defer conn.Close()

	// Read until newline or EOF.
	var buf []byte
	tmp := make([]byte, 1024)
	foundNL := false
	for !foundNL {
		n, err := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for _, b := range buf[len(buf)-n:] {
				if b == '\n' {
					foundNL = true
					break
				}
			}
		}
		if err != nil {
			break
		}
	}

	// Trim to first line.
	line := string(buf)
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	// Parse request.
	var request struct {
		ID     int            `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &request); err != nil {
		writeJSONResponse(conn, 0, nil, fmt.Errorf("invalid request: %w", err))
		return
	}

	result, err := HandleRequest(ctx, ops, request.Method, request.Params)
	writeJSONResponse(conn, request.ID, result, err)
}

// writeJSONResponse marshals a JSON-RPC-style response and writes it
// to conn, followed by a newline.
func writeJSONResponse(conn net.Conn, id int, result any, err error) {
	var resp map[string]any
	if err != nil {
		resp = map[string]any{"id": id, "error": err.Error()}
	} else {
		resp = map[string]any{"id": id, "result": result}
	}
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	conn.Write(b)
}

// RunDaemon is the daemon entrypoint. It is invoked as:
//
//	mcp2cli __session-daemon <configJSON>
func RunDaemon(configJSON string) error {
	var cfg DaemonConfig
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return fmt.Errorf("parsing daemon config: %w", err)
	}

	// Set up context with signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		cancel()
	}()

	// Connect to the MCP server.
	client, err := mcp.Connect(ctx, cfg.Source, cfg.IsStdio, cfg.AuthHeaders, cfg.EnvVars, cfg.Transport)
	if err != nil {
		return fmt.Errorf("connecting to MCP server: %w", err)
	}
	defer client.Close()

	// Determine transport label for metadata.
	transportLabel := "http"
	if cfg.IsStdio {
		transportLabel = "stdio"
	}

	// Write metadata.
	meta := Meta{
		Pid:       os.Getpid(),
		Source:    cfg.Source,
		Transport: transportLabel,
		CreatedAt: float64(time.Now().Unix()),
	}
	metaJSON, _ := json.Marshal(meta)
	mp := metaPath(cfg.Name)
	os.MkdirAll(SessionsDir(), 0755)
	if err := os.WriteFile(mp, metaJSON, 0644); err != nil {
		return fmt.Errorf("writing metadata: %w", err)
	}

	// Remove any stale socket.
	sp := sockPath(cfg.Name)
	os.Remove(sp)

	// Listen on Unix domain socket.
	ln, err := net.Listen("unix", sp)
	if err != nil {
		os.Remove(mp)
		return fmt.Errorf("listening on socket: %w", err)
	}

	// Accept loop in a goroutine.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					continue
				}
			}
			go ServeConn(ctx, client, conn)
		}
	}()

	// Block until context is done.
	<-ctx.Done()

	// Cleanup.
	ln.Close()
	os.Remove(sp)
	os.Remove(mp)
	return nil
}
