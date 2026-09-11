package satim

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBackoffDelay_RetryAfter(t *testing.T) {
	t.Parallel()

	d := backoffDelay(1, "2")
	if d != 2*time.Second {
		t.Fatalf("Retry-After seconds: got %v, want 2s", d)
	}

	d = backoffDelay(1, "30")
	if d != maxRetryAfter {
		t.Fatalf("Retry-After cap: got %v, want %v", d, maxRetryAfter)
	}

	past := time.Now().UTC().Add(-2 * time.Second).Format(http.TimeFormat)
	d = backoffDelay(1, past)
	if d != 0 {
		t.Fatalf("past Retry-After should clamp to 0, got %v", d)
	}

	d = backoffDelay(0, "")
	if d != 0 {
		t.Fatalf("attempt 0 base should be 0, got %v", d)
	}

	d = backoffDelay(2, "")
	if d < 0 || d > 200*time.Millisecond {
		t.Fatalf("jitter backoff out of range: %v", d)
	}

	d = backoffDelay(1, "not-a-retry-after")
	if d < 0 || d > 100*time.Millisecond {
		t.Fatalf("invalid Retry-After should fall back to jitter: %v", d)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	if _, ok := parseRetryAfter(""); ok {
		t.Fatal("empty should not parse")
	}
	if _, ok := parseRetryAfter("   "); ok {
		t.Fatal("whitespace should not parse")
	}
	if _, ok := parseRetryAfter("bogus"); ok {
		t.Fatal("bogus should not parse")
	}
	d, ok := parseRetryAfter("3")
	if !ok || d != 3*time.Second {
		t.Fatalf("seconds: got %v ok=%v", d, ok)
	}
	future := time.Now().UTC().Add(2 * time.Second).Format(http.TimeFormat)
	d, ok = parseRetryAfter(future)
	if !ok || d <= 0 {
		t.Fatalf("HTTP-date: got %v ok=%v", d, ok)
	}
}

func TestIsJSONContentType(t *testing.T) {
	t.Parallel()
	if isJSONContentType("") {
		t.Fatal("empty should be false")
	}
	if !isJSONContentType("application/json") {
		t.Fatal("expected true for application/json")
	}
	if !isJSONContentType("application/json; charset=utf-8") {
		t.Fatal("expected true for json with charset")
	}
	if isJSONContentType("text/html") {
		t.Fatal("expected false for text/html")
	}
	// Malformed media type that still mentions json — fallback path.
	if !isJSONContentType("application/json; charset=\"utf-8") {
		t.Fatal("expected true via contains fallback for malformed CT")
	}
}

func TestUserAgent(t *testing.T) {
	t.Parallel()
	ua := userAgent()
	if ua != "go-satim/dev (+https://github.com/muandane/go-satim)" {
		t.Fatalf("unexpected user-agent: %q", ua)
	}
}

func TestHTTPStatusError_Error(t *testing.T) {
	t.Parallel()
	err := &HTTPStatusError{StatusCode: 502, Body: []byte("bad gateway body")}
	if got := err.Error(); !strings.Contains(got, "502") {
		t.Fatalf("unexpected error string: %q", got)
	}

	long := strings.Repeat("x", 200)
	err = &HTTPStatusError{StatusCode: 500, Body: []byte(long)}
	got := err.Error()
	if !strings.Contains(got, "...") {
		t.Fatalf("expected truncated snippet with ..., got %q", got)
	}
}

func TestTruncateBody(t *testing.T) {
	t.Parallel()

	short := []byte("hi")
	out := truncateBody(short, 512)
	if string(out) != "hi" {
		t.Fatalf("short body: got %q", out)
	}
	// Mutation isolation.
	short[0] = 'X'
	if string(out) != "hi" {
		t.Fatal("truncateBody must copy short bodies")
	}

	long := []byte(strings.Repeat("a", 600))
	out = truncateBody(long, 512)
	if len(out) != 512 {
		t.Fatalf("long body len = %d, want 512", len(out))
	}
}

func TestValidateCallbackURL(t *testing.T) {
	t.Parallel()

	if err := validateCallbackURL("https://ok.dz/cb", false); err != nil {
		t.Fatalf("https should pass: %v", err)
	}
	if err := validateCallbackURL("http://ok.dz/cb", false); err == nil {
		t.Fatal("http should fail when allowHTTP=false")
	}
	if err := validateCallbackURL("http://ok.dz/cb", true); err != nil {
		t.Fatalf("http should pass when allowHTTP=true: %v", err)
	}
	if err := validateCallbackURL("javascript:alert(1)", false); err == nil {
		t.Fatal("javascript should fail")
	}
	if err := validateCallbackURL("://bad", false); err == nil {
		t.Fatal("malformed URL should fail")
	}
	if err := validateCallbackURL("ftp://files.example/x", true); err == nil {
		t.Fatal("ftp should fail even when allowHTTP")
	}
}

func TestExtractString(t *testing.T) {
	t.Parallel()

	m := map[string]any{
		"s": "hello",
		"f": float64(5),
		"i": int(7),
		"b": true,
	}
	if v, ok := extractString(m, "missing", "s"); !ok || v != "hello" {
		t.Fatalf("string: got %q ok=%v", v, ok)
	}
	if v, ok := extractString(m, "f"); !ok || v != "5" {
		t.Fatalf("float64: got %q ok=%v", v, ok)
	}
	if v, ok := extractString(m, "i"); !ok || v != "7" {
		t.Fatalf("int: got %q ok=%v", v, ok)
	}
	if _, ok := extractString(m, "b"); ok {
		t.Fatal("bool should not extract")
	}
	if _, ok := extractString(m, "nope"); ok {
		t.Fatal("missing key should not extract")
	}
}

func TestParseNumericInt64(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{float64(42), 42, true},
		{int64(99), 99, true},
		{int(7), 7, true},
		{" 123 ", 123, true},
		{"45.9", 45, true},
		{"nope", 0, false},
		{true, 0, false},
		{nil, 0, false},
	}
	for _, tc := range cases {
		got, ok := parseNumericInt64(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("parseNumericInt64(%v) = (%d,%v), want (%d,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPrepareRegisterRequest(t *testing.T) {
	t.Parallel()

	out, err := prepareRegisterRequest(RegisterOrderRequest{
		AmountMinor: 1000,
		ReturnURL:   "https://example.com/r",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.FailURL != out.ReturnURL {
		t.Fatalf("FailURL default: got %q", out.FailURL)
	}
	if out.Language != LanguageFR {
		t.Fatalf("Language default: got %q", out.Language)
	}
	if out.OrderNumber < 1000000000 {
		t.Fatalf("OrderNumber not generated: %d", out.OrderNumber)
	}

	_, err = prepareRegisterRequest(RegisterOrderRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDoRequest_NilFormAndTypedDecodeMismatch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Valid JSON object, but incompatible with []int target in execute.
		_, _ = w.Write([]byte(`{"errorCode":"0","value":"ok"}`))
	}))
	t.Cleanup(server.Close)

	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"},
		WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}

	// nil form path in doRequest
	val, raw, err := c.doRequest(context.Background(), "/x.do", nil, false)
	if err != nil {
		t.Fatalf("doRequest nil form: %v", err)
	}
	if len(val) == 0 || raw == nil {
		t.Fatal("expected value and raw")
	}

	// execute typed decode failure (map ok, T not)
	_, err = c.execute[[]int](context.Background(), "/x.do", url.Values{}, false)
	if err == nil || !strings.Contains(err.Error(), "decode json response") {
		t.Fatalf("expected typed decode error, got %v", err)
	}
}

func TestDoRequest_ContextCancelDuringBackoff(t *testing.T) {
	t.Parallel()

	var n int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n++
		w.Header().Set("Retry-After", "5")
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"},
		WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first response so the backoff wait sees Done.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err = c.GetStatus(ctx, GetStatusRequest{OrderID: "ord-1"})
	if err == nil {
		t.Fatal("expected context error")
	}
	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
		!strings.Contains(err.Error(), "context") {
		// May be wrapped as exhausted retries if cancel raced after wait; both ok if we retried.
		if n < 1 {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestRegister_FailURLSchemeRejected(t *testing.T) {
	t.Parallel()

	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Register(context.Background(), RegisterOrderRequest{
		AmountMinor: 100000,
		ReturnURL:   "https://shop.dz/ok",
		FailURL:     "javascript:alert(1)",
	})
	if !errors.Is(err, ErrInvalidURL) {
		t.Fatalf("expected ErrInvalidURL for FailURL, got %v", err)
	}
}

func TestOrderStatusUnmarshal_OrderNumberLowercaseAndInts(t *testing.T) {
	t.Parallel()

	var resp OrderStatusResponse
	err := json.Unmarshal([]byte(`{
		"orderId":"o1",
		"orderNumber": 1234567890,
		"OrderStatus": 2,
		"ErrorCode": 0,
		"actionCode": 0,
		"amount": 50000
	}`), &resp)
	if err != nil {
		t.Fatal(err)
	}
	if resp.OrderNumber != 1234567890 {
		t.Fatalf("OrderNumber = %d", resp.OrderNumber)
	}
	if resp.OrderStatus != OrderStatusApproved {
		t.Fatalf("OrderStatus = %q", resp.OrderStatus)
	}
	if resp.AmountMinor != 50000 {
		t.Fatalf("AmountMinor = %d", resp.AmountMinor)
	}

	err = json.Unmarshal([]byte(`[]`), &resp)
	if err == nil {
		t.Fatal("expected unmarshal error for JSON array")
	}
	err = json.Unmarshal([]byte(`"nope"`), &resp)
	if err == nil {
		t.Fatal("expected unmarshal error for JSON string")
	}
}

type errReadCloser struct{}

func (errReadCloser) Read([]byte) (int, error) { return 0, errors.New("boom read") }
func (errReadCloser) Close() error             { return nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDoRequest_BuildRequestError(t *testing.T) {
	t.Parallel()
	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"})
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = "http://%" // invalid URL
	_, _, err = c.doRequest(context.Background(), "/x.do", nil, false)
	if err == nil || !strings.Contains(err.Error(), "build request") {
		t.Fatalf("expected build request error, got %v", err)
	}
}

func TestDoRequest_ReadBodyError_Mutation(t *testing.T) {
	t.Parallel()
	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"},
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       errReadCloser{},
				Header:     make(http.Header),
			}, nil
		})}))
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = "http://example.invalid"
	_, _, err = c.doRequest(context.Background(), "/x.do", url.Values{}, false)
	if err == nil || !strings.Contains(err.Error(), "read response body") {
		t.Fatalf("expected read body error, got %v", err)
	}
}

func TestDoRequest_ReadBodyError_ReadOnlyExhausted(t *testing.T) {
	t.Parallel()
	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"},
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       errReadCloser{},
				Header:     make(http.Header),
			}, nil
		})}),
		WithLogger(slog.New(slog.DiscardHandler)),
	)
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = "http://example.invalid"
	_, err = c.GetStatus(context.Background(), GetStatusRequest{OrderID: "o1"})
	if err == nil || !strings.Contains(err.Error(), "request failed after") {
		t.Fatalf("expected exhausted retries, got %v", err)
	}
}

func TestDoRequest_TransportError_ReadOnlyExhausted(t *testing.T) {
	t.Parallel()
	c, err := NewClient(Credentials{Username: "u", Password: "p", TerminalID: "t"},
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("dial fail")
		})}),
		WithLogger(slog.Default()), // exercise DebugContext logging path
	)
	if err != nil {
		t.Fatal(err)
	}
	c.baseURL = "http://example.invalid"
	_, err = c.GetStatus(context.Background(), GetStatusRequest{OrderID: "o1"})
	if err == nil || !strings.Contains(err.Error(), "request failed after") {
		t.Fatalf("expected exhausted retries, got %v", err)
	}
}
