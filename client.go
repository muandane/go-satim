package satim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	prodBaseURL = "https://cib.satim.dz/payment/rest"
	testBaseURL = "https://test.satim.dz/payment/rest"

	defaultHTTPTimeout = 15 * time.Second
	maxResponseBody    = 1 << 20 // 1 MB response limit to prevent memory exhaustion
	readOnlyMaxRetries = 2
)

// Client interacts with the SATIM / BPC REST payment gateway.
type Client struct {
	creds      Credentials
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates and initializes a new SATIM API Client.
func NewClient(creds Credentials, opts ...ClientOption) (*Client, error) {
	if err := creds.Validate(); err != nil {
		return nil, err
	}

	c := &Client{
		creds:   creds,
		baseURL: prodBaseURL,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		logger: slog.New(slog.DiscardHandler),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}

	return c, nil
}

// Credentials returns the configured credentials with password redacted.
func (c *Client) Credentials() Credentials {
	return Credentials{
		Username:   c.creds.Username,
		Password:   "[REDACTED]",
		TerminalID: c.creds.TerminalID,
	}
}

// BaseURL returns the configured base API URL.
func (c *Client) BaseURL() string {
	return c.baseURL
}

type rawSettable interface {
	setRaw(map[string]any)
}

// Do performs an HTTP POST request to a SATIM endpoint and decodes the JSON response into T.
func (c *Client) Do[T any](ctx context.Context, endpoint string, form url.Values) (*T, error) {
	return c.execute[T](ctx, endpoint, form, false)
}

// execute executes an HTTP request, decodes the response into T, and assigns raw map data if supported.
func (c *Client) execute[T any](ctx context.Context, endpoint string, form url.Values, isReadOnly bool) (*T, error) {
	body, raw, err := c.doRequest(ctx, endpoint, form, isReadOnly)
	if err != nil {
		return nil, err
	}

	var resp T
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("satim: decode json response: %w", err)
	}

	if s, ok := any(&resp).(rawSettable); ok {
		s.setRaw(raw)
	}

	return &resp, nil
}

// doRequest performs an HTTP POST request to the SATIM REST gateway.
// When isReadOnly is true (e.g. for GetStatus), safe retries are applied on transient network errors.
func (c *Client) doRequest(ctx context.Context, endpoint string, form url.Values, isReadOnly bool) ([]byte, map[string]any, error) {
	if form == nil {
		form = make(url.Values)
	}
	form.Set("userName", c.creds.Username)
	form.Set("password", c.creds.Password)

	reqURL := c.baseURL + endpoint
	encodedBody := form.Encode()

	maxAttempts := 1
	if isReadOnly {
		maxAttempts = 1 + readOnlyMaxRetries
	}

	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(time.Duration(attempt*100) * time.Millisecond):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(encodedBody))
		if err != nil {
			return nil, nil, fmt.Errorf("satim: build request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		httpReq.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = err
			c.logger.DebugContext(ctx, "satim request failed",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", attempt+1),
				slog.String("error", err.Error()),
			)
			if !isReadOnly {
				return nil, nil, fmt.Errorf("satim: transport error: %w", err)
			}
			continue
		}

		limitedReader := io.LimitReader(resp.Body, maxResponseBody)
		body, err := io.ReadAll(limitedReader)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			if !isReadOnly {
				return nil, nil, fmt.Errorf("satim: read response body: %w", err)
			}
			continue
		}

		if resp.StatusCode >= http.StatusInternalServerError && isReadOnly && attempt < maxAttempts-1 {
			lastErr = fmt.Errorf("satim: server error HTTP %d", resp.StatusCode)
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, nil, fmt.Errorf("satim: decode json response: %w", err)
		}

		// Check for error codes in response
		if errorCode, ok := extractString(raw, "errorCode", "ErrorCode"); ok && errorCode != "0" && errorCode != "" {
			errorMsg, _ := extractString(raw, "errorMessage", "ErrorMessage")
			apiErr := &APIError{
				ErrorCode:    errorCode,
				ErrorMessage: errorMsg,
				HTTPStatus:   resp.StatusCode,
				Raw:          raw,
			}
			return nil, nil, apiErr
		}

		return body, raw, nil
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("satim: request failed after %d attempts: %w", maxAttempts, lastErr)
	}

	return nil, nil, errors.New("satim: request failed with unknown error")
}

func extractString(m map[string]any, keys ...string) (string, bool) {
	for _, k := range keys {
		if val, ok := m[k]; ok {
			switch v := val.(type) {
			case string:
				return v, true
			case float64:
				return fmt.Sprintf("%.0f", v), true
			case int:
				return fmt.Sprintf("%d", v), true
			}
		}
	}
	return "", false
}
