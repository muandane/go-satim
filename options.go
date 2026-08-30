package satim

import (
	"log/slog"
	"net/http"
	"strings"
)

// ClientOption configures a Client instance.
type ClientOption func(*Client)

// WithHTTPClient configures a custom http.Client for network requests.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// WithBaseURL overrides the API base URL. This is primarily intended for tests against a mock server.
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}
}

// WithTestMode switches between the production endpoint (cib.satim.dz) and the test sandbox (test.satim.dz).
func WithTestMode(enabled bool) ClientOption {
	return func(c *Client) {
		if enabled {
			c.baseURL = testBaseURL
		} else {
			c.baseURL = prodBaseURL
		}
	}
}

// WithLogger sets the structured logger for client operations.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) {
		if logger != nil {
			c.logger = logger
		}
	}
}
