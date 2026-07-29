package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// ExecuteGraphQL builds and executes a GraphQL query/mutation, writing the
// result to util.Out according to the given output options.
//
// If fieldsOverride is non-empty it overrides the auto-generated selection
// set. stdinVars, when non-nil, is used as the variables map directly
// (from --stdin).
func ExecuteGraphQL(
	cmd types.CommandDef,
	url string,
	schema map[string]any,
	authHeaders [][2]string,
	paramValues map[string]any,
	fieldsOverride string,
	stdinVars map[string]any,
	opts util.OutputOptions,
	client *http.Client,
) error {
	document, variables, fieldName := BuildDocument(cmd, schema, stdinVars, paramValues, fieldsOverride)

	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	body := map[string]any{
		"query": document,
	}
	if len(variables) > 0 {
		body["variables"] = variables
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	for _, h := range authHeaders {
		req.Header.Set(h[0], h[1])
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GraphQL request returned HTTP %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read GraphQL response: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	if errs, hasErrors := result["errors"]; hasErrors {
		if _, hasData := result["data"]; !hasData {
			msgs := extractErrorMessages(errs)
			return fmt.Errorf("GraphQL error: %s", strings.Join(msgs, "; "))
		}
		// Partial errors — include them in output.
		util.OutputResult(result, opts)
		return nil
	}

	data, _ := result["data"].(map[string]any)
	if data == nil {
		data = make(map[string]any)
	}
	fieldData := data[fieldName]
	if fieldData == nil {
		fieldData = data
	}
	util.OutputResult(fieldData, opts)
	return nil
}
