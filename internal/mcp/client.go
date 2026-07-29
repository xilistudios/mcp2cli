package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ---------------------------------------------------------------------------
// ContentPart / CallResult
// ---------------------------------------------------------------------------

// ContentPart is a single content element returned by CallTool.
type ContentPart struct {
	Type string // "text", "image", etc.
	Text string // non-empty for text content
	Data string // base64 blob for binary content (images, audio, etc.)
}

// CallResult is the simplified result of a tool call.
type CallResult struct {
	Content           []ContentPart
	StructuredContent map[string]any
	IsError           bool
}

// Text joins all content parts with "\n", mirroring Python
// _extract_content_parts. Text parts use their Text field; binary
// parts use their Data (base64 blob).
func (r *CallResult) Text() string {
	if r == nil || len(r.Content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(r.Content))
	for _, c := range r.Content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		} else if c.Data != "" {
			parts = append(parts, c.Data)
		}
	}
	return strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// Client wrapper
// ---------------------------------------------------------------------------

// Client wraps an mcp-go client with a transport-agnostic API.
type Client struct {
	inner *mcpgo.Client
}

// Close releases the underlying client resources.
func (c *Client) Close() error {
	if c.inner != nil {
		return c.inner.Close()
	}
	return nil
}

// Connect establishes and initializes an MCP session.
//
// source: URL (http/https) or command string (for stdio).
// isStdio: if true, source is treated as a shell command.
// authHeaders: key-value pairs added as HTTP headers.
// envVars: extra environment variables for stdio mode.
// transport: "auto" (try streamable then SSE), "streamable", "sse".
//
//	Ignored for stdio.
func Connect(
	ctx context.Context,
	source string,
	isStdio bool,
	authHeaders [][2]string,
	envVars map[string]string,
	transport string,
) (*Client, error) {
	if isStdio {
		return connectStdio(ctx, source, envVars)
	}
	return connectHTTP(ctx, source, authHeaders, transport)
}

// connectStdio creates a stdio MCP client from a command string.
func connectStdio(ctx context.Context, commandStr string, envVars map[string]string) (*Client, error) {
	parts, err := splitCommand(commandStr)
	if err != nil {
		return nil, fmt.Errorf("invalid command: %w", err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command string")
	}

	// Merge environment: os.Environ + extra envVars.
	env := os.Environ()
	for k, v := range envVars {
		env = append(env, k+"="+v)
	}

	c, err := mcpgo.NewStdioMCPClient(parts[0], env, parts[1:]...)
	if err != nil {
		return nil, fmt.Errorf("creating stdio client: %w", err)
	}

	// NewStdioMCPClient auto-starts, but we still need to Initialize.
	if err := initClient(ctx, c); err != nil {
		c.Close()
		return nil, err
	}

	return &Client{inner: c}, nil
}

// connectHTTP creates an HTTP-based MCP client.
func connectHTTP(
	ctx context.Context,
	baseURL string,
	authHeaders [][2]string,
	transport string,
) (*Client, error) {
	headers := make(map[string]string, len(authHeaders))
	for _, h := range authHeaders {
		headers[h[0]] = h[1]
	}

	switch transport {
	case "sse":
		return connectSSE(ctx, baseURL, headers)
	case "streamable":
		return connectStreamable(ctx, baseURL, headers)
	default: // "auto"
		c, err := connectStreamable(ctx, baseURL, headers)
		if err == nil {
			return c, nil
		}
		// Fall back to SSE.
		return connectSSE(ctx, baseURL, headers)
	}
}

// connectStreamable creates a streamable HTTP client and initializes it.
func connectStreamable(ctx context.Context, baseURL string, headers map[string]string) (*Client, error) {
	var opts []mcptransport.StreamableHTTPCOption
	if len(headers) > 0 {
		opts = append(opts, mcptransport.WithHTTPHeaders(headers))
	}

	c, err := mcpgo.NewStreamableHttpClient(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating streamable HTTP client: %w", err)
	}

	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("starting streamable client: %w", err)
	}

	if err := initClient(ctx, c); err != nil {
		c.Close()
		return nil, err
	}

	return &Client{inner: c}, nil
}

// connectSSE creates an SSE client and initializes it.
func connectSSE(ctx context.Context, baseURL string, headers map[string]string) (*Client, error) {
	var opts []mcptransport.ClientOption
	if len(headers) > 0 {
		opts = append(opts, mcpgo.WithHeaders(headers))
	}

	c, err := mcpgo.NewSSEMCPClient(baseURL, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating SSE client: %w", err)
	}

	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("starting SSE client: %w", err)
	}

	if err := initClient(ctx, c); err != nil {
		c.Close()
		return nil, err
	}

	return &Client{inner: c}, nil
}

// initClient sends the Initialize request.
func initClient(ctx context.Context, c *mcpgo.Client) error {
	_, err := c.Initialize(ctx, mcpmcp.InitializeRequest{
		Params: mcpmcp.InitializeParams{
			ProtocolVersion: mcpmcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcpmcp.Implementation{
				Name:    "mcp2cli",
				Version: "0.1.0",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("initializing MCP session: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Client methods — adapt mcp-go types into simple map/slice shapes
// ---------------------------------------------------------------------------

// ListTools returns tool descriptors as maps with keys
// "name", "description", "inputSchema".
func (c *Client) ListTools(ctx context.Context) ([]map[string]any, error) {
	result, err := c.inner.ListTools(ctx, mcpmcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(result.Tools))
	for _, t := range result.Tools {
		// Convert InputSchema to map[string]any.
		schema := toolSchemaToMap(t.InputSchema)
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		})
	}
	return out, nil
}

// CallTool invokes a named tool and returns the result.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (*CallResult, error) {
	result, err := c.inner.CallTool(ctx, mcpmcp.CallToolRequest{
		Params: mcpmcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})
	if err != nil {
		return nil, err
	}

	cr := &CallResult{IsError: result.IsError}

	for _, content := range result.Content {
		part := contentToPart(content)
		cr.Content = append(cr.Content, part)
	}

	// StructuredContent
	if sc, ok := result.StructuredContent.(map[string]any); ok {
		cr.StructuredContent = sc
	}

	return cr, nil
}

// ListResources returns resource descriptors as maps.
func (c *Client) ListResources(ctx context.Context) ([]map[string]any, error) {
	result, err := c.inner.ListResources(ctx, mcpmcp.ListResourcesRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(result.Resources))
	for _, r := range result.Resources {
		out = append(out, map[string]any{
			"name":        r.Name,
			"uri":         r.URI,
			"description": r.Description,
			"mimeType":    r.MIMEType,
		})
	}
	return out, nil
}

// ReadResource reads a resource by URI and returns concatenated text/blob content.
func (c *Client) ReadResource(ctx context.Context, uri string) (string, error) {
	result, err := c.inner.ReadResource(ctx, mcpmcp.ReadResourceRequest{
		Params: mcpmcp.ReadResourceParams{
			URI: uri,
		},
	})
	if err != nil {
		return "", err
	}

	var parts []string
	for _, content := range result.Contents {
		if tc, ok := mcpmcp.AsTextResourceContents(content); ok {
			parts = append(parts, tc.Text)
		} else if bc, ok := mcpmcp.AsBlobResourceContents(content); ok {
			parts = append(parts, bc.Blob)
		}
	}
	return strings.Join(parts, "\n"), nil
}

// ListResourceTemplates returns resource template descriptors as maps.
func (c *Client) ListResourceTemplates(ctx context.Context) ([]map[string]any, error) {
	result, err := c.inner.ListResourceTemplates(ctx, mcpmcp.ListResourceTemplatesRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(result.ResourceTemplates))
	for _, t := range result.ResourceTemplates {
		uriTmpl := ""
		if t.URITemplate != nil {
			uriTmpl = t.URITemplate.Raw()
		}
		out = append(out, map[string]any{
			"name":        t.Name,
			"uriTemplate": uriTmpl,
			"description": t.Description,
			"mimeType":    t.MIMEType,
		})
	}
	return out, nil
}

// ListPrompts returns prompt descriptors as maps.
func (c *Client) ListPrompts(ctx context.Context) ([]map[string]any, error) {
	result, err := c.inner.ListPrompts(ctx, mcpmcp.ListPromptsRequest{})
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(result.Prompts))
	for _, p := range result.Prompts {
		args := make([]map[string]any, 0, len(p.Arguments))
		for _, a := range p.Arguments {
			args = append(args, map[string]any{
				"name":        a.Name,
				"description": a.Description,
				"required":    a.Required,
			})
		}
		out = append(out, map[string]any{
			"name":        p.Name,
			"description": p.Description,
			"arguments":   args,
		})
	}
	return out, nil
}

// GetPrompt retrieves a prompt by name with optional arguments.
func (c *Client) GetPrompt(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	// Convert map[string]any to map[string]string for the MCP SDK.
	strArgs := make(map[string]string, len(arguments))
	for k, v := range arguments {
		strArgs[k] = fmt.Sprintf("%v", v)
	}

	result, err := c.inner.GetPrompt(ctx, mcpmcp.GetPromptRequest{
		Params: mcpmcp.GetPromptParams{
			Name:      name,
			Arguments: strArgs,
		},
	})
	if err != nil {
		return nil, err
	}

	messages := make([]map[string]any, 0, len(result.Messages))
	for _, msg := range result.Messages {
		content := mcpmcp.GetTextFromContent(msg.Content)
		messages = append(messages, map[string]any{
			"role":    string(msg.Role),
			"content": content,
		})
	}

	return map[string]any{
		"description": result.Description,
		"messages":    messages,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// contentToPart converts an mcp-go Content interface into a ContentPart.
func contentToPart(c mcpmcp.Content) ContentPart {
	if tc, ok := mcpmcp.AsTextContent(c); ok {
		return ContentPart{Type: "text", Text: tc.Text}
	}
	if ic, ok := mcpmcp.AsImageContent(c); ok {
		return ContentPart{Type: "image", Data: ic.Data}
	}
	if ac, ok := mcpmcp.AsAudioContent(c); ok {
		return ContentPart{Type: "audio", Data: ac.Data}
	}
	// Fallback: use GetTextFromContent.
	return ContentPart{Type: "text", Text: mcpmcp.GetTextFromContent(c)}
}

// toolSchemaToMap converts a ToolInputSchema to a plain map[string]any
// for the CLI extraction layer.
func toolSchemaToMap(schema mcpmcp.ToolInputSchema) map[string]any {
	out := map[string]any{
		"type": schema.Type,
	}
	if schema.Properties != nil {
		out["properties"] = schema.Properties
	}
	if len(schema.Required) > 0 {
		req := make([]any, len(schema.Required))
		for i, r := range schema.Required {
			req[i] = r
		}
		out["required"] = req
	}
	return out
}

// splitCommand performs minimal shell-like splitting of a command string,
// respecting single and double quotes. This is a simplified shlex.
func splitCommand(s string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range s {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case r == ' ' && !inSingle && !inDouble:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in command string")
	}
	return parts, nil
}

// NewInProcessClient creates a Client wrapping an in-process mcp-go server.
// It starts and initializes the client automatically.
// This is used for testing.
func NewInProcessClient(ctx context.Context, server *mcpserver.MCPServer) (*Client, error) {
	c, err := mcpgo.NewInProcessClient(server)
	if err != nil {
		return nil, err
	}

	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("starting in-process client: %w", err)
	}

	if err := initClient(ctx, c); err != nil {
		c.Close()
		return nil, err
	}

	return &Client{inner: c}, nil
}
