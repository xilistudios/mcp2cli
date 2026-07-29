package mcp

import (
	"context"
	"fmt"
	"testing"

	mcpmcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/xilistudios/mcp2cli/internal/types"
)

// ---------------------------------------------------------------------------
// ExtractCommands tests (pure, no server needed)
// ---------------------------------------------------------------------------

func TestExtractCommands_Basic(t *testing.T) {
	tools := []map[string]any{
		{
			"name":        "get_weather",
			"description": "Get current weather for a city",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"city": map[string]any{
						"type":        "string",
						"description": "City name",
					},
					"units": map[string]any{
						"type":        "string",
						"description": "Temperature units",
						"enum":        []any{"celsius", "fahrenheit"},
					},
					"includeDetails": map[string]any{
						"type":        "boolean",
						"description": "Include detailed forecast",
					},
					"maxResults": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results",
					},
					"tags": map[string]any{
						"type":        "array",
						"description": "Filter tags",
					},
				},
				"required": []any{"city"},
			},
		},
	}

	cmds := ExtractCommands(tools)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}

	cmd := cmds[0]

	// Name should be kebab-case.
	if cmd.Name != "get-weather" {
		t.Errorf("expected name 'get-weather', got %q", cmd.Name)
	}

	// ToolName preserved.
	if cmd.ToolName != "get_weather" {
		t.Errorf("expected ToolName 'get_weather', got %q", cmd.ToolName)
	}

	if cmd.Description != "Get current weather for a city" {
		t.Errorf("unexpected description: %q", cmd.Description)
	}

	if !cmd.HasBody {
		t.Error("expected HasBody=true when there are params")
	}

	if len(cmd.Params) != 5 {
		t.Fatalf("expected 5 params, got %d", len(cmd.Params))
	}

	// Check each param by OriginalName.
	byOrig := map[string]types.ParamDef{}
	for _, p := range cmd.Params {
		byOrig[p.OriginalName] = p
	}

	// city: required string
	city := byOrig["city"]
	if !city.Required {
		t.Error("city should be required")
	}
	if city.Type != types.TypeString {
		t.Errorf("city type = %q, want str", city.Type)
	}
	if city.Name != "city" {
		t.Errorf("city kebab name = %q, want 'city'", city.Name)
	}
	if city.Location != "tool_input" {
		t.Errorf("city location = %q, want 'tool_input'", city.Location)
	}
	if city.Description != "City name" {
		t.Errorf("city description = %q", city.Description)
	}

	// units: optional string with enum
	units := byOrig["units"]
	if units.Required {
		t.Error("units should not be required")
	}
	if units.Choices == nil || len(units.Choices) != 2 {
		t.Errorf("units choices = %v, want [celsius fahrenheit]", units.Choices)
	}

	// includeDetails -> include-details (kebab)
	details := byOrig["includeDetails"]
	if details.Name != "include-details" {
		t.Errorf("includeDetails kebab = %q, want 'include-details'", details.Name)
	}
	if details.Type != types.TypeBoolean {
		t.Errorf("includeDetails type = %q, want boolean", details.Type)
	}

	// maxResults: integer
	maxR := byOrig["maxResults"]
	if maxR.Name != "max-results" {
		t.Errorf("maxResults kebab = %q, want 'max-results'", maxR.Name)
	}
	if maxR.Type != types.TypeInt {
		t.Errorf("maxResults type = %q, want int", maxR.Type)
	}

	// tags: array -> type str with suffix
	tags := byOrig["tags"]
	if tags.Type != types.TypeString {
		t.Errorf("tags type = %q, want str", tags.Type)
	}
	if tags.Description != "Filter tags (JSON array)" {
		t.Errorf("tags description = %q", tags.Description)
	}
}

func TestExtractCommands_EmptySchema(t *testing.T) {
	tools := []map[string]any{
		{
			"name":        "noop",
			"description": "Does nothing",
		},
	}

	cmds := ExtractCommands(tools)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}

	cmd := cmds[0]
	if cmd.HasBody {
		t.Error("expected HasBody=false for empty schema")
	}
	if len(cmd.Params) != 0 {
		t.Errorf("expected 0 params, got %d", len(cmd.Params))
	}
}

func TestExtractCommands_MissingName(t *testing.T) {
	tools := []map[string]any{
		{
			"description": "Unknown tool",
		},
	}

	cmds := ExtractCommands(tools)
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}

	if cmds[0].Name != "unknown" {
		t.Errorf("expected name 'unknown', got %q", cmds[0].Name)
	}
}

func TestExtractCommands_MultipleTools(t *testing.T) {
	tools := []map[string]any{
		{"name": "alpha_tool", "description": "A"},
		{"name": "betaTool", "description": "B"},
		{"name": "gamma-thing", "description": "C"},
	}

	cmds := ExtractCommands(tools)
	if len(cmds) != 3 {
		t.Fatalf("expected 3, got %d", len(cmds))
	}

	expected := []string{"alpha-tool", "beta-tool", "gamma-thing"}
	for i, want := range expected {
		if cmds[i].Name != want {
			t.Errorf("cmd[%d].Name = %q, want %q", i, cmds[i].Name, want)
		}
	}
}

// ---------------------------------------------------------------------------
// CallResult.Text() tests
// ---------------------------------------------------------------------------

func TestCallResultText_Simple(t *testing.T) {
	r := &CallResult{
		Content: []ContentPart{
			{Type: "text", Text: "hello"},
			{Type: "text", Text: "world"},
		},
	}
	if got := r.Text(); got != "hello\nworld" {
		t.Errorf("got %q, want 'hello\\nworld'", got)
	}
}

func TestCallResultText_Mixed(t *testing.T) {
	r := &CallResult{
		Content: []ContentPart{
			{Type: "text", Text: "some text"},
			{Type: "image", Data: "base64data"},
		},
	}
	if got := r.Text(); got != "some text\nbase64data" {
		t.Errorf("got %q, want 'some text\\nbase64data'", got)
	}
}

func TestCallResultText_Empty(t *testing.T) {
	r := &CallResult{}
	if got := r.Text(); got != "" {
		t.Errorf("got %q, want ''", got)
	}
}

func TestCallResultText_Nil(t *testing.T) {
	var r *CallResult
	if got := r.Text(); got != "" {
		t.Errorf("got %q, want ''", got)
	}
}

func TestCallResultText_SkipEmptyParts(t *testing.T) {
	r := &CallResult{
		Content: []ContentPart{
			{Type: "text", Text: "a"},
			{Type: "image", Data: ""}, // empty, should be skipped
			{Type: "text", Text: "b"},
		},
	}
	if got := r.Text(); got != "a\nb" {
		t.Errorf("got %q, want 'a\\nb'", got)
	}
}

// ---------------------------------------------------------------------------
// splitCommand tests
// ---------------------------------------------------------------------------

func TestSplitCommand_Basic(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"echo hello", []string{"echo", "hello"}},
		{"cmd arg1 arg2", []string{"cmd", "arg1", "arg2"}},
		{`cmd "hello world"`, []string{"cmd", "hello world"}},
		{`cmd 'hello world'`, []string{"cmd", "hello world"}},
		{`cmd hello\ world`, []string{"cmd", "hello world"}},
		{"  spaced   out  ", []string{"spaced", "out"}},
	}

	for _, tt := range tests {
		got, err := splitCommand(tt.input)
		if err != nil {
			t.Errorf("splitCommand(%q) error: %v", tt.input, err)
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("splitCommand(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitCommand(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestSplitCommand_UnclosedQuote(t *testing.T) {
	_, err := splitCommand(`cmd "hello`)
	if err == nil {
		t.Error("expected error for unclosed quote")
	}
}

// ---------------------------------------------------------------------------
// Integration test with in-process MCP server
// ---------------------------------------------------------------------------

func TestIntegration_InProcessServer(t *testing.T) {
	// Build a test MCP server with tools, a resource, and a prompt.
	srv := mcpserver.NewMCPServer("test-server", "1.0.0")

	// Add "echo" tool.
	echoTool := mcpmcp.NewTool("echo",
		mcpmcp.WithDescription("Echo back the input message"),
		mcpmcp.WithString("message", mcpmcp.Required(), mcpmcp.Description("Message to echo")),
	)
	srv.AddTool(echoTool, func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		msg := req.GetArguments()["message"]
		return mcpmcp.NewToolResultText(fmt.Sprintf("%v", msg)), nil
	})

	// Add "add" tool.
	addTool := mcpmcp.NewTool("add",
		mcpmcp.WithDescription("Add two numbers"),
		mcpmcp.WithInteger("a", mcpmcp.Required(), mcpmcp.Description("First number")),
		mcpmcp.WithInteger("b", mcpmcp.Required(), mcpmcp.Description("Second number")),
	)
	srv.AddTool(addTool, func(ctx context.Context, req mcpmcp.CallToolRequest) (*mcpmcp.CallToolResult, error) {
		args := req.GetArguments()
		a, _ := args["a"].(float64)
		b, _ := args["b"].(float64)
		return mcpmcp.NewToolResultText(fmt.Sprintf("%v", int(a+b))), nil
	})

	// Add a resource.
	res := mcpmcp.NewResource("test://data", "test-data",
		mcpmcp.WithResourceDescription("A test resource"),
		mcpmcp.WithMIMEType("text/plain"),
	)
	srv.AddResource(res, func(ctx context.Context, req mcpmcp.ReadResourceRequest) ([]mcpmcp.ResourceContents, error) {
		return []mcpmcp.ResourceContents{
			mcpmcp.TextResourceContents{
				URI:  "test://data",
				Text: "resource-content",
			},
		}, nil
	})

	// Add a prompt.
	prompt := mcpmcp.NewPrompt("greet",
		mcpmcp.WithPromptDescription("A greeting prompt"),
		mcpmcp.WithArgument("name",
			mcpmcp.RequiredArgument(),
			mcpmcp.ArgumentDescription("Name to greet"),
		),
	)
	srv.AddPrompt(prompt, func(ctx context.Context, req mcpmcp.GetPromptRequest) (*mcpmcp.GetPromptResult, error) {
		name := req.Params.Arguments["name"]
		return mcpmcp.NewGetPromptResult("A greeting prompt", []mcpmcp.PromptMessage{
			mcpmcp.NewPromptMessage(mcpmcp.RoleUser, mcpmcp.NewTextContent("Hello, "+name+"!")),
		}), nil
	})

	// Connect using in-process client.
	ctx := context.Background()
	c, err := NewInProcessClient(ctx, srv)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer c.Close()

	// --- ListTools ---
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) < 2 {
		t.Fatalf("expected >=2 tools, got %d", len(tools))
	}

	toolNames := map[string]bool{}
	for _, t := range tools {
		toolNames[t["name"].(string)] = true
	}
	if !toolNames["echo"] || !toolNames["add"] {
		t.Errorf("expected echo and add tools, got %v", toolNames)
	}

	// --- CallTool echo ---
	echoResult, err := c.CallTool(ctx, "echo", map[string]any{"message": "hi"})
	if err != nil {
		t.Fatalf("CallTool echo: %v", err)
	}
	if echoResult.Text() != "hi" {
		t.Errorf("echo result = %q, want 'hi'", echoResult.Text())
	}

	// --- CallTool add ---
	addResult, err := c.CallTool(ctx, "add", map[string]any{"a": float64(2), "b": float64(3)})
	if err != nil {
		t.Fatalf("CallTool add: %v", err)
	}
	if addResult.Text() != "5" {
		t.Errorf("add result = %q, want '5'", addResult.Text())
	}

	// --- ListResources ---
	resources, err := c.ListResources(ctx)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources) == 0 {
		t.Fatal("expected >=1 resource")
	}
	found := false
	for _, r := range resources {
		if r["uri"] == "test://data" {
			found = true
			if r["name"] != "test-data" {
				t.Errorf("resource name = %q, want 'test-data'", r["name"])
			}
		}
	}
	if !found {
		t.Error("expected resource test://data not found")
	}

	// --- ReadResource ---
	content, err := c.ReadResource(ctx, "test://data")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if content != "resource-content" {
		t.Errorf("ReadResource content = %q, want 'resource-content'", content)
	}

	// --- ListPrompts ---
	prompts, err := c.ListPrompts(ctx)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	if len(prompts) == 0 {
		t.Fatal("expected >=1 prompt")
	}
	foundGreet := false
	for _, p := range prompts {
		if p["name"] == "greet" {
			foundGreet = true
			if p["description"] != "A greeting prompt" {
				t.Errorf("prompt description = %q", p["description"])
			}
		}
	}
	if !foundGreet {
		t.Error("expected prompt 'greet' not found")
	}

	// --- GetPrompt ---
	promptResult, err := c.GetPrompt(ctx, "greet", map[string]any{"name": "World"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	if promptResult["description"] != "A greeting prompt" {
		t.Errorf("GetPrompt description = %q", promptResult["description"])
	}
	messages, ok := promptResult["messages"].([]map[string]any)
	if !ok {
		t.Fatalf("messages type = %T", promptResult["messages"])
	}
	if len(messages) == 0 {
		t.Fatal("expected >=1 message")
	}
	if messages[0]["content"] != "Hello, World!" {
		t.Errorf("prompt message = %q, want 'Hello, World!'", messages[0]["content"])
	}
}
