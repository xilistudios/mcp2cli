package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
)

// callbackResult carries the authorization code and state from the HTTP handler.
type callbackResult struct {
	code, state string
}

// CallbackServer runs a local HTTP server that receives the OAuth redirect callback.
type CallbackServer struct {
	srv     *http.Server
	ln      net.Listener
	results chan callbackResult
	done    chan struct{}
}

// Addr returns the listener's address (host:port).
func (c *CallbackServer) Addr() string { return c.ln.Addr().String() }

// StartCallbackServer starts a local HTTP server on the host and port parsed from redirectURI.
// If the URI has no port, it defaults to 3334.
func StartCallbackServer(redirectURI string) (*CallbackServer, error) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		return nil, fmt.Errorf("parsing redirect URI: %w", err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "3334"
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}

	results := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		select {
		case results <- callbackResult{code: code, state: state}:
		default:
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h2>Authorization complete.</h2><p>You can close this window and return to the terminal.</p></body></html>`)
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	return &CallbackServer{
		srv:     srv,
		ln:      ln,
		results: results,
		done:    make(chan struct{}),
	}, nil
}

// WaitForCode blocks until the callback receives a code or ctx expires.
func (c *CallbackServer) WaitForCode(ctx context.Context) (code, state string, err error) {
	select {
	case r := <-c.results:
		return r.code, r.state, nil
	case <-ctx.Done():
		return "", "", ctx.Err()
	}
}

// Close shuts down the callback server.
func (c *CallbackServer) Close() error { return c.srv.Close() }
