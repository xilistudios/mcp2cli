package openapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/xilistudios/mcp2cli/internal/types"
	"github.com/xilistudios/mcp2cli/internal/util"
)

// FilePart represents a file to upload in a multipart request.
type FilePart struct {
	Filename string
	Open     func() (io.ReadCloser, error)
	MIME     string
}

// Request is the fully-resolved HTTP request ready for execution.
type Request struct {
	Method      string
	URL         string
	Query       map[string]any
	Headers     map[string]string
	Body        map[string]any      // nil when there is no body
	Files       map[string]FilePart // nil when there are no files
	ContentType string              // "" → JSON, "multipart/form-data", etc.
}

// BuildRequest assembles a Request from a CommandDef, a base URL, and
// the user-supplied argument map (keyed by ParamDef.Name in kebab-case).
// hasStdin indicates whether stdin JSON was provided.
func BuildRequest(
	cmd types.CommandDef,
	baseURL string,
	args map[string]any,
	hasStdin bool,
	stdinJSON any,
) (Request, error) {
	path := cmd.Path

	// Substitute path parameters.
	for _, p := range cmd.Params {
		if p.Location == "path" {
			if v, ok := args[p.Name]; ok && v != nil {
				path = strings.ReplaceAll(path, "{"+p.OriginalName+"}", fmt.Sprint(v))
			}
		}
	}

	query := map[string]any{}
	extraHeaders := map[string]string{}
	var body map[string]any
	var files map[string]FilePart

	if cmd.Method == "get" {
		for _, p := range cmd.Params {
			v := args[p.Name]
			if v == nil {
				continue
			}
			switch p.Location {
			case "query":
				query[p.OriginalName] = util.CoerceValue(v, p.Schema)
			case "header":
				extraHeaders[p.OriginalName] = fmt.Sprint(v)
			}
		}
	} else {
		if hasStdin {
			if m, ok := stdinJSON.(map[string]any); ok {
				body = m
			}
		} else {
			body = map[string]any{}
			for _, p := range cmd.Params {
				v := args[p.Name]
				switch p.Location {
				case "header":
					if v != nil {
						extraHeaders[p.OriginalName] = fmt.Sprint(v)
					}
					continue
				case "path":
					continue
				case "file":
					if v != nil {
						fp := fmt.Sprint(v)
						fi, err := os.Stat(fp)
						if err != nil || fi.IsDir() {
							return Request{}, fmt.Errorf("file not found: %s", fp)
						}
						mimeType := guessMIME(fp)
						if files == nil {
							files = map[string]FilePart{}
						}
						filePath := fp // capture for closure
						files[p.OriginalName] = FilePart{
							Filename: filepath.Base(fp),
							Open: func() (io.ReadCloser, error) {
								return os.Open(filePath)
							},
							MIME: mimeType,
						}
					}
					continue
				default: // body
					if v != nil {
						body[p.OriginalName] = util.CoerceValue(v, p.Schema)
					}
				}
			}
			if len(body) == 0 {
				body = nil
			}
		}

		// Also collect query params for non-GET.
		for _, p := range cmd.Params {
			if p.Location == "query" {
				if v := args[p.Name]; v != nil {
					query[p.OriginalName] = util.CoerceValue(v, p.Schema)
				}
			}
		}
	}

	fullURL := strings.TrimRight(baseURL, "/") + path
	return Request{
		Method:      cmd.Method,
		URL:         fullURL,
		Query:       query,
		Headers:     extraHeaders,
		Body:        body,
		Files:       files,
		ContentType: cmd.ContentType,
	}, nil
}

// Execute sends the HTTP request described by req and writes the response
// to util.Out according to out options.
func Execute(req Request, authHeaders [][2]string, out util.OutputOptions, client *http.Client) error {
	isMultipart := req.Files != nil || req.ContentType == "multipart/form-data"

	// Build headers.
	headers := map[string]string{}
	for _, h := range authHeaders {
		headers[h[0]] = h[1]
	}
	if !isMultipart {
		if _, ok := headers["Content-Type"]; !ok {
			headers["Content-Type"] = "application/json"
		}
	}
	for k, v := range req.Headers {
		headers[k] = v
	}

	// Build body and content-type.
	var bodyReader io.Reader
	var contentType string

	if req.Files != nil {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		// Write file parts.
		for field, fp := range req.Files {
			fw, err := mw.CreateFormFile(field, fp.Filename)
			if err != nil {
				return fmt.Errorf("multipart form file: %w", err)
			}
			rc, err := fp.Open()
			if err != nil {
				return fmt.Errorf("opening file %s: %w", fp.Filename, err)
			}
			io.Copy(fw, rc)
			rc.Close()
		}
		// Write body fields.
		for k, v := range req.Body {
			var s string
			switch v.(type) {
			case string:
				s = v.(string)
			default:
				b, _ := json.Marshal(v)
				s = string(b)
			}
			mw.WriteField(k, s)
		}
		mw.Close()
		bodyReader = &buf
		contentType = mw.FormDataContentType()

	} else if req.ContentType == "multipart/form-data" {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for k, v := range req.Body {
			var s string
			switch v.(type) {
			case string:
				s = v.(string)
			default:
				b, _ := json.Marshal(v)
				s = string(b)
			}
			mw.WriteField(k, s)
		}
		mw.Close()
		bodyReader = &buf
		contentType = mw.FormDataContentType()

	} else {
		if req.Body != nil {
			b, err := json.Marshal(req.Body)
			if err != nil {
				return fmt.Errorf("marshaling body: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}
	}

	// Build URL with query parameters.
	fullURL := req.URL
	if len(req.Query) > 0 {
		q := url.Values{}
		for k, v := range req.Query {
			encodeQueryValue(q, k, v)
		}
		fullURL += "?" + q.Encode()
	}

	method := strings.ToUpper(req.Method)
	if method == "" {
		method = "GET"
	}

	httpReq, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	if client == nil {
		client = &http.Client{Timeout: 60 * 1e9} // 60s
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("%d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Output handling.
	if out.JSONOutput {
		data := tryParseJSON(bodyBytes)
		util.OutputResult(data, util.OutputOptions{
			Pretty:     out.Pretty,
			Head:       out.Head,
			JSONOutput: true,
		})
		return nil
	}

	if out.Raw {
		util.Out.Write(bodyBytes)
		return nil
	}

	data := tryParseJSON(bodyBytes)
	if s, ok := data.(string); ok {
		fmt.Fprintln(util.Out, s)
		return nil
	}
	util.OutputResult(data, util.OutputOptions{
		Pretty: out.Pretty,
		Toon:   out.Toon,
		Head:   out.Head,
	})
	return nil
}

// --- helpers ---

func guessMIME(path string) string {
	ext := filepath.Ext(path)
	if ext != "" {
		if m := mime.TypeByExtension(ext); m != "" {
			return m
		}
	}
	return "application/octet-stream"
}

func tryParseJSON(b []byte) any {
	var parsed any
	if json.Unmarshal(b, &parsed) == nil {
		return parsed
	}
	return string(b)
}

func encodeQueryValue(q url.Values, key string, val any) {
	switch v := val.(type) {
	case []any:
		for _, elem := range v {
			q.Add(key, encodeQueryScalar(elem))
		}
	default:
		q.Set(key, encodeQueryScalar(v))
	}
}

func encodeQueryScalar(v any) string {
	switch t := v.(type) {
	case map[string]any:
		b, _ := json.Marshal(t)
		return string(b)
	case []any:
		b, _ := json.Marshal(t)
		return string(b)
	default:
		return fmt.Sprint(v)
	}
}
