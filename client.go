package satim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	prodBaseURL = "https://cib.satim.dz/payment/rest"
	testBaseURL = "https://test.satim.dz/payment/rest"

	defaultHTTPTimeout = 15 * time.Second
	maxResponseBody    = 1 << 20 // 1 MB response limit to prevent memory exhaustion
	readOnlyMaxRetries = 2
	maxRetryAfter      = 5 * time.Second
	httpStatusBodyCap  = 512
)

// Version is the SDK version embedded in the User-Agent header.
// Override at link time, for example:
//
//	go build -ldflags "-X github.com/muandane/go-satim.Version=v1.2.3"
var Version = "dev"

func userAgent() string {
	return "go-satim/" + Version + " (+https://github.com/muandane/go-satim)"
}

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
//
// Do is an authenticated low-level escape hatch: it performs no automatic retries.
// Prefer the typed package methods (Register, Confirm, GetStatus, Refund) unless you need
// a custom SATIM/BPC endpoint that this SDK does not expose.
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
	var retryAfterHeader string

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := backoffDelay(attempt, retryAfterHeader)
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		retryAfterHeader = ""

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(encodedBody))
		if err != nil {
			return nil, nil, fmt.Errorf("satim: build request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		httpReq.Header.Set("Accept", "application/json")
		httpReq.Header.Set("User-Agent", userAgent())

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
			retryAfterHeader = resp.Header.Get("Retry-After")
			lastErr = fmt.Errorf("satim: server error HTTP %d", resp.StatusCode)
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return nil, nil, &HTTPStatusError{
				StatusCode: resp.StatusCode,
				Body:       truncateBody(body, httpStatusBodyCap),
			}
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

func isJSONContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return strings.Contains(strings.ToLower(contentType), "application/json")
	}
	return mediaType == "application/json"
}

// backoffDelay returns the wait before a retry attempt.
// Full jitter over [0, attempt*100ms], overridden by Retry-After when present (capped at 5s).
func backoffDelay(attempt int, retryAfter string) time.Duration {
	if d, ok := parseRetryAfter(retryAfter); ok {
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		if d < 0 {
			return 0
		}
		return d
	}
	base := time.Duration(attempt*100) * time.Millisecond
	if base <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(base) + 1))
}

func parseRetryAfter(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		return d, true
	}
	return 0, false
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
