package satim_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/muandane/go-satim"
)

func TestClient_Confirm(t *testing.T) {
	t.Parallel()

	creds := satim.Credentials{Username: "user", Password: "pwd", TerminalID: "TERM01"}

	t.Run("validation empty order ID", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.Confirm(t.Context(), satim.ConfirmRequest{})
		if !errors.Is(err, satim.ErrMissingRequiredData) {
			t.Fatalf("expected ErrMissingRequiredData, got %v", err)
		}
	})

	t.Run("validation invalid language", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.Confirm(t.Context(), satim.ConfirmRequest{
			OrderID:  "ord-123",
			Language: "ES",
		})
		if !errors.Is(err, satim.ErrInvalidLanguage) {
			t.Fatalf("expected ErrInvalidLanguage, got %v", err)
		}
	})

	t.Run("successful confirmation", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/confirmOrder.do" {
				t.Errorf("expected /confirmOrder.do, got %s", r.URL.Path)
			}
			_ = r.ParseForm()
			if r.FormValue("orderId") != "ord-123" {
				t.Errorf("expected orderId ord-123, got %s", r.FormValue("orderId"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"orderId": "ord-123",
				"OrderStatus": "2",
				"ErrorCode": "0",
				"actionCode": "0",
				"actionCodeDescription": "Approved",
				"amount": "150000",
				"currency": "012",
				"Pan": "628058******1234",
				"cardholderName": "JOHN DOE",
				"expiration": "202712",
				"approvalCode": "APP987",
				"Ip": "197.200.10.5",
				"params": {
					"respCode_desc": "Transaction Successful"
				}
			}`))
		})

		resp, err := client.Confirm(t.Context(), satim.ConfirmRequest{
			OrderID:  "ord-123",
			Language: satim.LanguageFR,
		})
		if err != nil {
			t.Fatalf("Confirm failed: %v", err)
		}

		if resp.OrderID != "ord-123" {
			t.Errorf("expected OrderID ord-123, got %s", resp.OrderID)
		}
		if !resp.IsSuccessful() {
			t.Errorf("expected IsSuccessful() == true")
		}
		if resp.IsPending() {
			t.Errorf("expected IsPending() == false")
		}
		if resp.AmountMinor != 150000 {
			t.Errorf("expected amount 150000, got %d", resp.AmountMinor)
		}
		if resp.MaskedPAN() != "628058******1234" {
			t.Errorf("expected masked PAN, got %s", resp.MaskedPAN())
		}
		if resp.CardholderName != "JOHN DOE" {
			t.Errorf("expected cardholder JOHN DOE, got %s", resp.CardholderName)
		}
		if resp.ApprovalCode != "APP987" {
			t.Errorf("expected approval code APP987, got %s", resp.ApprovalCode)
		}
		if resp.IP != "197.200.10.5" {
			t.Errorf("expected IP 197.200.10.5, got %s", resp.IP)
		}
		if resp.SuccessMessage() != "Transaction Successful" {
			t.Errorf("expected success message, got %s", resp.SuccessMessage())
		}
	})
}

func TestClient_GetStatus(t *testing.T) {
	t.Parallel()

	creds := satim.Credentials{Username: "user", Password: "pwd", TerminalID: "TERM01"}

	t.Run("validation empty order ID", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.GetStatus(t.Context(), satim.GetStatusRequest{})
		if !errors.Is(err, satim.ErrMissingRequiredData) {
			t.Fatalf("expected ErrMissingRequiredData, got %v", err)
		}
	})

	t.Run("validation invalid language", func(t *testing.T) {
		t.Parallel()
		client, _ := satim.NewClient(creds)
		_, err := client.GetStatus(t.Context(), satim.GetStatusRequest{
			OrderID:  "ord-123",
			Language: "ES",
		})
		if !errors.Is(err, satim.ErrInvalidLanguage) {
			t.Fatalf("expected ErrInvalidLanguage, got %v", err)
		}
	})

	t.Run("order not found error 6", func(t *testing.T) {
		t.Parallel()
		client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ErrorCode":"6","ErrorMessage":"Unknown order id"}`))
		})

		_, err := client.GetStatus(t.Context(), satim.GetStatusRequest{OrderID: "missing-ord"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, satim.ErrOrderNotFound) {
			t.Fatalf("expected ErrOrderNotFound, got %v", err)
		}
	})
}

func TestOrderStatusResponse_Predicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		resp         satim.OrderStatusResponse
		isSuccess    bool
		isPending    bool
		isRejected   bool
		isRefunded   bool
		isCancelled  bool
		isExpired    bool
		isFailed     bool
		wantSuccessM string
		wantErrorM   string
	}{
		{
			name: "registered order status 0 (unpaid/pending, NEVER successful)",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusRegistered,
				ErrorCode:   "0",
			},
			isSuccess:   false,
			isPending:   true,
			isRejected:  false,
			isRefunded:  false,
			isCancelled: false,
			isExpired:   false,
			isFailed:    false,
			wantErrorM:  "Payment failed",
		},
		{
			name: "3D-Secure authentication in progress status 5",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusACSAuth,
				ErrorCode:   "0",
			},
			isSuccess:   false,
			isPending:   true,
			isRejected:  false,
			isRefunded:  false,
			isCancelled: false,
			isExpired:   false,
			isFailed:    false,
			wantErrorM:  "Payment failed",
		},
		{
			name: "successful approved order status 2",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusApproved,
				ErrorCode:   "0",
				ActionCode:  "0",
				Params: map[string]string{
					"respCode_desc": "Approved by Bank",
				},
			},
			isSuccess:    true,
			isPending:    false,
			isRejected:   false,
			isRefunded:   false,
			isCancelled:  false,
			isExpired:    false,
			isFailed:     false,
			wantSuccessM: "Approved by Bank",
		},
		{
			name: "declined order status 3",
			resp: satim.OrderStatusResponse{
				OrderStatus:      satim.OrderStatusDeclined,
				ErrorCode:        "0",
				ErrorMessageText: "Payment is declined",
			},
			isSuccess:    false,
			isPending:    false,
			isRejected:   true,
			isRefunded:   false,
			isCancelled:  false,
			isExpired:    false,
			isFailed:     true,
			wantSuccessM: "« Votre transaction a été rejetée/ Your transaction was rejected/ تم رفض معاملتك »",
			wantErrorM:   "« Votre transaction a été rejetée/ Your transaction was rejected/ تم رفض معاملتك »",
		},
		{
			name: "auth failed order status 6",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusAuthFailed,
				ErrorCode:   "0",
			},
			isSuccess:    false,
			isPending:    false,
			isRejected:   true,
			isRefunded:   false,
			isCancelled:  false,
			isExpired:    false,
			isFailed:     true,
			wantSuccessM: "« Votre transaction a été rejetée/ Your transaction was rejected/ تم رفض معاملتك »",
			wantErrorM:   "« Votre transaction a été rejetée/ Your transaction was rejected/ تم رفض معاملتك »",
		},
		{
			name: "refunded order status 4",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusRefunded,
				ErrorCode:   "0",
			},
			isSuccess:  false,
			isPending:  false,
			isRejected: false,
			isRefunded: true,
			isFailed:   false,
			wantErrorM: "Payment was refunded",
		},
		{
			name: "cancelled action code 10",
			resp: satim.OrderStatusResponse{
				ActionCode:       "10",
				ErrorMessageText: "Payment is cancelled by user",
			},
			isCancelled: true,
			isPending:   false,
			isSuccess:   false,
			isFailed:    true,
		},
		{
			name: "expired action code -2007",
			resp: satim.OrderStatusResponse{
				ActionCode: "-2007",
			},
			isExpired: true,
			isPending: false,
			isSuccess: false,
			isFailed:  true,
		},
		{
			name: "action code 2003 declined",
			resp: satim.OrderStatusResponse{
				ActionCode: "2003",
			},
			isRejected: true,
			isPending:  false,
			isSuccess:  false,
			isFailed:   true,
		},
		{
			name: "successful order with ActionCodeDescription fallback",
			resp: satim.OrderStatusResponse{
				OrderStatus:           satim.OrderStatusApproved,
				ErrorCode:             "0",
				ActionCodeDescription: "Success from action code",
			},
			isSuccess:    true,
			wantSuccessM: "Success from action code",
		},
		{
			name: "successful order with default message",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusApproved,
				ErrorCode:   "0",
			},
			isSuccess:    true,
			wantSuccessM: "Payment was successful",
		},
		{
			name: "error message with respCode_desc param",
			resp: satim.OrderStatusResponse{
				OrderStatus: satim.OrderStatusRegistered,
				Params: map[string]string{
					"respCode_desc": "Custom error param desc",
				},
			},
			isPending:  true,
			wantErrorM: "Custom error param desc",
		},
		{
			name: "error message with actionCodeDescription",
			resp: satim.OrderStatusResponse{
				OrderStatus:           satim.OrderStatusRegistered,
				ActionCodeDescription: "Action code failed",
			},
			isPending:  true,
			wantErrorM: "Action code failed",
		},
		{
			name: "error message with ErrorMessageText",
			resp: satim.OrderStatusResponse{
				OrderStatus:      satim.OrderStatusRegistered,
				ErrorMessageText: "Text error from gateway",
			},
			isPending:  true,
			wantErrorM: "Text error from gateway",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.resp.IsSuccessful(); got != tc.isSuccess {
				t.Errorf("IsSuccessful() = %v, want %v", got, tc.isSuccess)
			}
			if got := tc.resp.IsPending(); got != tc.isPending {
				t.Errorf("IsPending() = %v, want %v", got, tc.isPending)
			}
			if got := tc.resp.IsRejected(); got != tc.isRejected {
				t.Errorf("IsRejected() = %v, want %v", got, tc.isRejected)
			}
			if got := tc.resp.IsRefunded(); got != tc.isRefunded {
				t.Errorf("IsRefunded() = %v, want %v", got, tc.isRefunded)
			}
			if got := tc.resp.IsCancelled(); got != tc.isCancelled {
				t.Errorf("IsCancelled() = %v, want %v", got, tc.isCancelled)
			}
			if got := tc.resp.IsExpired(); got != tc.isExpired {
				t.Errorf("IsExpired() = %v, want %v", got, tc.isExpired)
			}
			if got := tc.resp.IsFailed(); got != tc.isFailed {
				t.Errorf("IsFailed() = %v, want %v", got, tc.isFailed)
			}
			if tc.wantSuccessM != "" {
				if got := tc.resp.SuccessMessage(); got != tc.wantSuccessM {
					t.Errorf("SuccessMessage() = %q, want %q", got, tc.wantSuccessM)
				}
			}
			if tc.wantErrorM != "" {
				if got := tc.resp.ErrorMessage(); got != tc.wantErrorM {
					t.Errorf("ErrorMessage() = %q, want %q", got, tc.wantErrorM)
				}
			}
		})
	}
}
