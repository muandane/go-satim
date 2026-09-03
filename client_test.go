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
