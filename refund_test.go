package satim_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muandane/go-satim"
)

func TestClient_Refund(t *testing.T) {
	t.Parallel()

	creds := satim.Credentials{Username: "merchant", Password: "pwd", TerminalID: "TERM01"}

	t.Run("validation empty order ID", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.Refund(context.Background(), satim.RefundRequest{
			OrderID:     "",
			AmountMinor: 50000,
		})
		if !errors.Is(err, satim.ErrMissingRequiredData) {
			t.Fatalf("expected ErrMissingRequiredData, got %v", err)
		}
	})

	t.Run("validation invalid amount", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.Refund(context.Background(), satim.RefundRequest{
			OrderID:     "ord-123",
			AmountMinor: 0,
		})
		if !errors.Is(err, satim.ErrInvalidAmount) {
			t.Fatalf("expected ErrInvalidAmount, got %v", err)
		}
	})

	t.Run("validation invalid language", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.Refund(context.Background(), satim.RefundRequest{
			OrderID:     "ord-123",
			AmountMinor: 50000,
			Language:    "IT",
		})
		if !errors.Is(err, satim.ErrInvalidLanguage) {
			t.Fatalf("expected ErrInvalidLanguage, got %v", err)
		}
	})

	t.Run("successful refund", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/refund.do" {
				t.Errorf("expected /refund.do, got %s", r.URL.Path)
			}
			_ = r.ParseForm()
			if r.FormValue("orderId") != "ord-123" {
				t.Errorf("expected orderId ord-123, got %s", r.FormValue("orderId"))
			}
			if r.FormValue("amount") != "50000" {
				t.Errorf("expected amount 50000, got %s", r.FormValue("amount"))
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errorCode":"0","errorMessage":"Success"}`))
		}))
		defer server.Close()

		client, err := satim.NewClient(creds, satim.WithBaseURL(server.URL), satim.WithHTTPClient(server.Client()))
		if err != nil {
			t.Fatalf("failed to create client: %v", err)
		}

		resp, err := client.Refund(context.Background(), satim.RefundRequest{
			OrderID:     "ord-123",
			AmountMinor: 50000, // 500.00 DZD
		})
		if err != nil {
			t.Fatalf("Refund failed: %v", err)
		}
		if resp.ErrorCode != "0" {
			t.Errorf("expected errorCode 0, got %s", resp.ErrorCode)
		}
	})
}
