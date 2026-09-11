package satim_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/muandane/go-satim"
)

func TestNewClient_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		creds   satim.Credentials
		wantErr error
	}{
		{
			name:    "empty username",
			creds:   satim.Credentials{Username: "", Password: "p", TerminalID: "t"},
			wantErr: satim.ErrMissingRequiredData,
		},
		{
			name:    "empty password",
			creds:   satim.Credentials{Username: "u", Password: "", TerminalID: "t"},
			wantErr: satim.ErrMissingRequiredData,
		},
		{
			name:    "empty terminal ID",
			creds:   satim.Credentials{Username: "u", Password: "p", TerminalID: ""},
			wantErr: satim.ErrMissingRequiredData,
		},
		{
			name:    "valid credentials",
			creds:   satim.Credentials{Username: "u", Password: "p", TerminalID: "t"},
			wantErr: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, err := satim.NewClient(tc.creds)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				if client != nil {
					t.Fatal("expected nil client on error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if client == nil {
					t.Fatal("expected non-nil client")
				}
			}
		})
	}
}

func TestClient_Options(t *testing.T) {
	t.Parallel()

	creds := satim.Credentials{Username: "user", Password: "secret", TerminalID: "term123"}

	t.Run("default options", func(t *testing.T) {
		t.Parallel()
		c, err := satim.NewClient(creds)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.BaseURL() != "https://cib.satim.dz/payment/rest" {
			t.Errorf("expected prod URL, got %s", c.BaseURL())
		}
	})

	t.Run("test mode option", func(t *testing.T) {
		t.Parallel()
		c, err := satim.NewClient(creds, satim.WithTestMode(true))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.BaseURL() != "https://test.satim.dz/payment/rest" {
			t.Errorf("expected test URL, got %s", c.BaseURL())
		}

		cProd, err := satim.NewClient(creds, satim.WithTestMode(false))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cProd.BaseURL() != "https://cib.satim.dz/payment/rest" {
			t.Errorf("expected prod URL, got %s", cProd.BaseURL())
		}
	})

	t.Run("logger and http client options", func(t *testing.T) {
		t.Parallel()
		logger := slog.Default()
		customHTTPClient := &http.Client{}
		c, err := satim.NewClient(
			creds,
			satim.WithLogger(logger),
			satim.WithLogger(nil),
			satim.WithHTTPClient(nil),
			satim.WithHTTPClient(customHTTPClient),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c == nil {
			t.Fatal("expected non-nil client")
		}
	})

	t.Run("custom base URL option", func(t *testing.T) {
		t.Parallel()
		c, err := satim.NewClient(creds, satim.WithBaseURL("https://custom.gateway.dz/rest/"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.BaseURL() != "https://custom.gateway.dz/rest" {
			t.Errorf("expected trimmed custom URL, got %s", c.BaseURL())
		}
	})

	t.Run("later option wins when WithBaseURL and WithTestMode combined", func(t *testing.T) {
		t.Parallel()
		custom := "https://custom.gateway.dz/rest"

		c1, err := satim.NewClient(creds, satim.WithBaseURL(custom), satim.WithTestMode(true))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c1.BaseURL() != "https://test.satim.dz/payment/rest" {
			t.Errorf("expected test URL when WithTestMode is last, got %s", c1.BaseURL())
		}

		c2, err := satim.NewClient(creds, satim.WithTestMode(true), satim.WithBaseURL(custom))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c2.BaseURL() != custom {
			t.Errorf("expected custom URL when WithBaseURL is last, got %s", c2.BaseURL())
		}
	})
}

func TestCredentials_Redaction(t *testing.T) {
	t.Parallel()

	creds := satim.Credentials{
		Username:   "merchant_admin",
		Password:   "super_secret_password",
		TerminalID: "998877",
	}

	t.Run("fmt.Stringer and GoStringer", func(t *testing.T) {
		t.Parallel()
		str := creds.String()
		if strings.Contains(str, "super_secret_password") {
			t.Errorf("password leaked in String(): %s", str)
		}
		if !strings.Contains(str, "[REDACTED]") {
			t.Errorf("expected [REDACTED] in String(): %s", str)
		}

		goStr := fmt.Sprintf("%#v", creds)
		if strings.Contains(goStr, "super_secret_password") {
			t.Errorf("password leaked in GoString: %s", goStr)
		}
	})

	t.Run("slog.LogValuer", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, nil))

		logger.Info("credentials test", slog.Any("creds", creds))
		output := buf.String()

		if strings.Contains(output, "super_secret_password") {
			t.Errorf("password leaked in slog output: %s", output)
		}
		if !strings.Contains(output, "[REDACTED]") {
			t.Errorf("expected [REDACTED] in slog output: %s", output)
		}
	})

	t.Run("Client.Credentials() method redaction", func(t *testing.T) {
		t.Parallel()
		client, err := satim.NewClient(creds)
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}
		returnedCreds := client.Credentials()
		if returnedCreds.Password != "[REDACTED]" {
			t.Errorf("expected Password to be [REDACTED], got %q", returnedCreds.Password)
		}
		if returnedCreds.Username != "merchant_admin" {
			t.Errorf("expected Username merchant_admin, got %q", returnedCreds.Username)
		}
		if returnedCreds.TerminalID != "998877" {
			t.Errorf("expected TerminalID 998877, got %q", returnedCreds.TerminalID)
		}
	})
}

func TestClient_RetryPolicy(t *testing.T) {
	t.Parallel()

	t.Run("GetStatus retries on server error and succeeds", func(t *testing.T) {
		t.Parallel()
		var attempts atomic.Int32

		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			att := attempts.Add(1)
			if att == 1 {
				http.Error(w, `{"errorCode":"500"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ErrorCode":"0","OrderStatus":"2","orderId":"ord-123"}`))
		})

		resp, err := client.GetStatus(t.Context(), satim.GetStatusRequest{OrderID: "ord-123"})
		if err != nil {
			t.Fatalf("expected successful retry, got: %v", err)
		}
		if resp.OrderID != "ord-123" || !resp.IsSuccessful() {
			t.Errorf("unexpected status response: %+v", resp)
		}
		if attempts.Load() < 2 {
			t.Errorf("expected at least 2 attempts, got %d", attempts.Load())
		}
	})

	t.Run("Register does NOT auto-retry on 500 server error", func(t *testing.T) {
		t.Parallel()
		var attempts atomic.Int32

		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			http.Error(w, `{"errorCode":"500","errorMessage":"Gateway fault"}`, http.StatusInternalServerError)
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://merchant.dz/return",
		})
		if err == nil {
			t.Fatal("expected error on 500 status")
		}
		if attempts.Load() != 1 {
			t.Errorf("expected exactly 1 attempt for mutation, got %d", attempts.Load())
		}
	})

	t.Run("GetStatus exhausts retries on continuous 500", func(t *testing.T) {
		t.Parallel()
		var attempts atomic.Int32

		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			attempts.Add(1)
			http.Error(w, `server temporarily unavailable`, http.StatusServiceUnavailable)
		})

		_, err := client.GetStatus(t.Context(), satim.GetStatusRequest{OrderID: "ord-retry-fail"})
		if err == nil {
			t.Fatal("expected error after exhausted retries")
		}
		if attempts.Load() != 3 {
			t.Errorf("expected 3 retry attempts, got %d", attempts.Load())
		}
	})

	t.Run("Numeric error code parsing and invalid JSON response", func(t *testing.T) {
		t.Parallel()

		t.Run("numeric errorCode float64 in JSON", func(t *testing.T) {
			t.Parallel()
			client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"errorCode": 5, "errorMessage": "Invalid credentials"}`))
			})

			_, err := client.GetStatus(t.Context(), satim.GetStatusRequest{OrderID: "ord-num-err"})
			if err == nil {
				t.Fatal("expected error for numeric errorCode")
			}
			if !errors.Is(err, satim.ErrInvalidCredentials) {
				t.Errorf("expected ErrInvalidCredentials, got %v", err)
			}
		})

		t.Run("malformed non-JSON response", func(t *testing.T) {
			t.Parallel()
			client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = w.Write([]byte(`<html><body>Bad Gateway</body></html>`))
			})

			_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
				AmountMinor: 100000,
				ReturnURL:   "https://example.com/return",
			})
			if err == nil {
				t.Fatal("expected decode error for invalid json")
			}
		})
	})
}

func TestClient_Do_GenericMethod(t *testing.T) {
	t.Parallel()

	type CustomResponse struct {
		CustomID string `json:"customId"`
		Status   string `json:"status"`
	}

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/customEndpoint.do" {
			t.Errorf("expected /customEndpoint.do, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"customId":"cust-99","status":"ok","errorCode":"0"}`))
	})

	form := make(url.Values)
	form.Set("param1", "val1")

	resp, err := client.Do[CustomResponse](t.Context(), "/customEndpoint.do", form)
	if err != nil {
		t.Fatalf("Do[CustomResponse] failed: %v", err)
	}
	if resp.CustomID != "cust-99" || resp.Status != "ok" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestClient_RetryPolicy_ContextCancellation(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `500 error`, http.StatusInternalServerError)
	})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := client.GetStatus(ctx, satim.GetStatusRequest{OrderID: "ord-cancel"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestClient_TransportError_NoRetry(t *testing.T) {
	t.Parallel()

	// Client pointed at dead port to induce connection refused
	client, err := satim.NewClient(
		defaultTestCreds,
		satim.WithBaseURL("http://127.0.0.1:1"),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	_, err = client.Register(t.Context(), satim.RegisterOrderRequest{
		AmountMinor: 100000,
		ReturnURL:   "https://shop.dz/return",
	})
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "satim: transport error:") {
		t.Errorf("expected 'satim: transport error:' in error message, got: %v", err)
	}
}

func TestClient_HTTPStatusError(t *testing.T) {
	t.Parallel()

	t.Run("non-JSON 502 returns HTTPStatusError", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`<html><body>Bad Gateway</body></html>`))
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		var httpErr *satim.HTTPStatusError
		if !errors.As(err, &httpErr) {
			t.Fatalf("expected *HTTPStatusError, got %v", err)
		}
		if httpErr.StatusCode != http.StatusBadGateway {
			t.Errorf("StatusCode = %d, want 502", httpErr.StatusCode)
		}
		if !strings.Contains(string(httpErr.Body), "Bad Gateway") {
			t.Errorf("Body = %q, want snippet containing Bad Gateway", httpErr.Body)
		}
	})

	t.Run("non-JSON 200 returns HTTPStatusError", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not json at all`))
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		var httpErr *satim.HTTPStatusError
		if !errors.As(err, &httpErr) {
			t.Fatalf("expected *HTTPStatusError, got %v", err)
		}
		if httpErr.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want 200", httpErr.StatusCode)
		}
	})

	t.Run("JSON error body still returns APIError", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"errorCode":"5","errorMessage":"Access denied"}`))
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		if !errors.Is(err, satim.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
	})

	t.Run("503 schema-less JSON returns HTTPStatusError not success", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"down"}`))
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		var httpErr *satim.HTTPStatusError
		if !errors.As(err, &httpErr) {
			t.Fatalf("expected *HTTPStatusError, got %v", err)
		}
		if httpErr.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("StatusCode = %d, want 503", httpErr.StatusCode)
		}
	})

	t.Run("400 with SATIM errorCode still returns APIError", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorCode":"5","errorMessage":"Access denied"}`))
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		if !errors.Is(err, satim.ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials, got %v", err)
		}
		if _, ok := errors.AsType[*satim.HTTPStatusError](err); ok {
			t.Fatal("must not downgrade APIError to HTTPStatusError")
		}
	})

	t.Run("400 SATIM error with wrong Content-Type still APIError", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errorCode":"6","errorMessage":"Unknown order"}`))
		})

		_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		if !errors.Is(err, satim.ErrOrderNotFound) {
			t.Fatalf("Content-Type must not override valid SATIM body, got %v", err)
		}
	})

	t.Run("200 with SATIM success body still succeeds", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"orderId":"o1","formUrl":"https://test.satim.dz/p","errorCode":"0"}`))
		})

		resp, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.OrderID != "o1" {
			t.Errorf("OrderID = %q, want o1", resp.OrderID)
		}
	})

	t.Run("200 schema-less JSON still succeeds", func(t *testing.T) {
		// Choice: keep prior 2xx behavior — schema-less JSON on 200 is treated as success.
		// Callers receive a typed zero/partial value rather than HTTPStatusError.
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok","note":"no errorCode field"}`))
		})

		resp, err := client.Register(t.Context(), satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://shop.dz/return",
		})
		if err != nil {
			t.Fatalf("200 schema-less must remain success (unchanged 2xx behavior), got %v", err)
		}
		if resp == nil {
			t.Fatal("expected non-nil response")
		}
	})
}

func TestClient_UserAgentHeader(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if !strings.Contains(ua, "go-satim/") {
			t.Errorf("User-Agent = %q, want substring go-satim/", ua)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orderId":"o1","formUrl":"https://test.satim.dz/p","errorCode":"0"}`))
	})

	_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
		AmountMinor: 100000,
		ReturnURL:   "https://shop.dz/return",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

func TestClient_RetryAfterHeader(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		att := attempts.Add(1)
		if att == 1 {
			w.Header().Set("Retry-After", "0")
			http.Error(w, `server error`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ErrorCode":"0","OrderStatus":"2","orderId":"ord-retry"}`))
	})

	resp, err := client.GetStatus(t.Context(), satim.GetStatusRequest{OrderID: "ord-retry"})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !resp.IsSuccessful() {
		t.Errorf("expected successful status")
	}
	if attempts.Load() < 2 {
		t.Errorf("expected retry, got %d attempts", attempts.Load())
	}
}
