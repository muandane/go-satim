package satim

import (
	"net/http"
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

	d = backoffDelay(2, "")
	if d < 0 || d > 200*time.Millisecond {
		t.Fatalf("jitter backoff out of range: %v", d)
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	if _, ok := parseRetryAfter(""); ok {
		t.Fatal("empty should not parse")
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
	if !isJSONContentType("application/json") {
		t.Fatal("expected true for application/json")
	}
	if !isJSONContentType("application/json; charset=utf-8") {
		t.Fatal("expected true for json with charset")
	}
	if isJSONContentType("text/html") {
		t.Fatal("expected false for text/html")
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
	if got := err.Error(); got == "" {
		t.Fatal("empty error string")
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
}
