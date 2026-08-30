package satim_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/muandane/go-satim"
)

func TestRegister_Validation(t *testing.T) {
	t.Parallel()

	validReq := func() satim.RegisterOrderRequest {
		return satim.RegisterOrderRequest{
			AmountMinor: 100000,
			ReturnURL:   "https://example.com/return",
		}
	}

	tests := []struct {
		name    string
		modify  func(*satim.RegisterOrderRequest)
		wantErr error
	}{
		{
			name: "zero amount",
			modify: func(r *satim.RegisterOrderRequest) {
				r.AmountMinor = 0
			},
			wantErr: satim.ErrInvalidAmount,
		},
		{
			name: "negative amount",
			modify: func(r *satim.RegisterOrderRequest) {
				r.AmountMinor = -5000
			},
			wantErr: satim.ErrInvalidAmount,
		},
		{
			name: "missing return URL",
			modify: func(r *satim.RegisterOrderRequest) {
				r.ReturnURL = ""
			},
			wantErr: satim.ErrMissingRequiredData,
		},
		{
			name: "invalid return URL",
			modify: func(r *satim.RegisterOrderRequest) {
				r.ReturnURL = "://not-a-valid-url"
			},
			wantErr: satim.ErrInvalidURL,
		},
		{
			name: "invalid fail URL",
			modify: func(r *satim.RegisterOrderRequest) {
				r.FailURL = "not_a_valid_url"
			},
			wantErr: satim.ErrInvalidURL,
		},
		{
			name: "invalid order number too small",
			modify: func(r *satim.RegisterOrderRequest) {
				r.OrderNumber = 123456
			},
			wantErr: satim.ErrInvalidOrderNumber,
		},
		{
			name: "invalid order number too large",
			modify: func(r *satim.RegisterOrderRequest) {
				r.OrderNumber = 10000000000
			},
			wantErr: satim.ErrInvalidOrderNumber,
		},
		{
			name: "invalid language",
			modify: func(r *satim.RegisterOrderRequest) {
				r.Language = "DE"
			},
			wantErr: satim.ErrInvalidLanguage,
		},
		{
			name: "timeout too small (<600)",
			modify: func(r *satim.RegisterOrderRequest) {
				r.SessionTimeoutSecs = 500
			},
			wantErr: satim.ErrInvalidTimeout,
		},
		{
			name: "timeout too large (>86400)",
			modify: func(r *satim.RegisterOrderRequest) {
				r.SessionTimeoutSecs = 90000
			},
			wantErr: satim.ErrInvalidTimeout,
		},
		{
			name: "description too long",
			modify: func(r *satim.RegisterOrderRequest) {
				r.Description = strings.Repeat("A", 600)
			},
			wantErr: satim.ErrDescriptionTooLong,
		},
		{
			name:   "valid default request",
			modify: func(_ *satim.RegisterOrderRequest) {},
		},
		{
			name: "valid fully specified request",
			modify: func(r *satim.RegisterOrderRequest) {
				r.FailURL = "https://example.com/fail"
				r.Language = satim.LanguageAR
				r.OrderNumber = 1234567890
				r.Description = "Valid test description"
				r.SessionTimeoutSecs = 1800
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := validReq()
			tc.modify(&req)

			err := req.Validate()
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected validation error: %v", err)
				}
				if req.OrderNumber < 1000000000 || req.OrderNumber > 9999999999 {
					t.Errorf("expected 10-digit order number, got %d", req.OrderNumber)
				}
				if tc.name == "valid default request" {
					if req.FailURL != req.ReturnURL {
						t.Errorf("expected FailURL to default to ReturnURL (%q), got %q", req.ReturnURL, req.FailURL)
					}
					if req.Language != satim.LanguageFR {
						t.Errorf("expected default LanguageFR, got %q", req.Language)
					}
				}
			}
		})
	}
}

func TestGenerateOrderNumber(t *testing.T) {
	t.Parallel()

	for range 100 {
		n, err := satim.GenerateOrderNumber()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n < 1000000000 || n > 9999999999 {
			t.Fatalf("generated number %d out of 10-digit range [1000000000, 9999999999]", n)
		}
	}
}

func TestClient_Register_Success(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register.do" {
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}

		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}

		if r.FormValue("userName") != "test_user" || r.FormValue("password") != "test_password" {
			t.Errorf("incorrect credentials in form: user=%s, pass=%s", r.FormValue("userName"), r.FormValue("password"))
		}
		if r.FormValue("currency") != "012" {
			t.Errorf("expected DZD currency 012, got %s", r.FormValue("currency"))
		}
		if r.FormValue("amount") != "250000" {
			t.Errorf("expected amount 250000, got %s", r.FormValue("amount"))
		}
		if r.FormValue("orderNumber") != "1234567890" {
			t.Errorf("expected orderNumber 1234567890, got %s", r.FormValue("orderNumber"))
		}

		var jsonParams map[string]string
		if err := json.Unmarshal([]byte(r.FormValue("jsonParams")), &jsonParams); err != nil {
			t.Fatalf("failed to unmarshal jsonParams: %v", err)
		}
		if jsonParams["force_terminal_id"] != "TERM01" {
			t.Errorf("expected force_terminal_id TERM01, got %s", jsonParams["force_terminal_id"])
		}
		if jsonParams["custom_client_id"] != "user-42" {
			t.Errorf("expected custom_client_id user-42, got %s", jsonParams["custom_client_id"])
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"orderId": "bpc-order-987",
			"formUrl": "https://test.satim.dz/payment/merch/bpc-order-987",
			"errorCode": "0"
		}`))
	})

	req := satim.RegisterOrderRequest{
		AmountMinor:        250000, // 2500.00 DZD
		OrderNumber:        1234567890,
		ReturnURL:          "https://shop.dz/checkout/success",
		FailURL:            "https://shop.dz/checkout/failed",
		Description:        "E-commerce purchase",
		Language:           satim.LanguageAR,
		SessionTimeoutSecs: 1800,
		UserDefinedFields: map[string]string{
			"custom_client_id": "user-42",
		},
	}

	resp, err := client.Register(t.Context(), req)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if resp.OrderID != "bpc-order-987" {
		t.Errorf("expected orderId bpc-order-987, got %s", resp.OrderID)
	}
	if resp.FormURL != "https://test.satim.dz/payment/merch/bpc-order-987" {
		t.Errorf("expected FormURL, got %s", resp.FormURL)
	}
	if resp.ErrorCode != "0" {
		t.Errorf("expected errorCode 0, got %s", resp.ErrorCode)
	}
}

func TestClient_Register_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		respJSON   string
		matchError error
	}{
		{
			name:       "invalid credentials error code 5",
			respJSON:   `{"errorCode":"5","errorMessage":"Access denied"}`,
			matchError: satim.ErrInvalidCredentials,
		},
		{
			name:       "duplicate order error code 1",
			respJSON:   `{"errorCode":"1","errorMessage":"Order with this number already exists"}`,
			matchError: satim.ErrOrderAlreadyExists,
		},
		{
			name:       "system error code 7",
			respJSON:   `{"errorCode":"7","errorMessage":"System malfunction"}`,
			matchError: satim.ErrSystemError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.respJSON))
			})

			_, err := client.Register(t.Context(), satim.RegisterOrderRequest{
				AmountMinor: 100000,
				ReturnURL:   "https://merchant.dz/return",
			})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tc.matchError) {
				t.Fatalf("expected errors.Is match for %v, got %v", tc.matchError, err)
			}
		})
	}
}
