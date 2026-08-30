package satim_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
}

func TestClient_RetryPolicy(t *testing.T) {
	t.Parallel()

	creds := satim.Credentials{Username: "u", Password: "p", TerminalID: "t"}

	t.Run("GetStatus retries on server error and succeeds", func(t *testing.T) {
		t.Parallel()
		var attempts atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			att := attempts.Add(1)
			if att == 1 {
				http.Error(w, `{"errorCode":"500"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ErrorCode":"0","OrderStatus":"2","orderId":"ord-123"}`))
		}))
		defer server.Close()

		c, err := satim.NewClient(creds, satim.WithBaseURL(server.URL), satim.WithHTTPClient(server.Client()))
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := c.GetStatus(ctx, satim.GetStatusRequest{OrderID: "ord-123"})
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

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			http.Error(w, `{"errorCode":"500","errorMessage":"Gateway fault"}`, http.StatusInternalServerError)
		}))
		defer server.Close()

		c, err := satim.NewClient(creds, satim.WithBaseURL(server.URL), satim.WithHTTPClient(server.Client()))
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		_, err = c.Register(context.Background(), satim.RegisterOrderRequest{
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
}
