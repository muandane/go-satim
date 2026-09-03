package satim_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/muandane/go-satim"
)

// TestClient_PaymentLifecycle tests the complete end-to-end payment workflow:
// 1. Register order and receive form URL and order ID.
// 2. Cardholder completes payment, merchant calls Confirm.
// 3. Merchant verifies status via GetStatus.
// 4. Merchant issues full Refund and verifies refunded state.
func TestClient_PaymentLifecycle(t *testing.T) {
	t.Parallel()

	const (
		mockOrderID     = "satim-order-uuid-999"
		mockOrderNumber = int64(1000200030)
		mockAmount      = int64(250000) // 2500.00 DZD
	)

	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = r.ParseForm()

		switch r.URL.Path {
		case "/register.do":
			if r.FormValue("orderNumber") != fmt.Sprintf("%d", mockOrderNumber) {
				http.Error(w, `{"errorCode":"1","errorMessage":"Order mismatch"}`, http.StatusBadRequest)
				return
			}
			fmt.Fprintf(w, `{
				"orderId": %q,
				"formUrl": "https://test.satim.dz/payment/merch/%s",
				"errorCode": "0"
			}`, mockOrderID, mockOrderID)

		case "/confirmOrder.do":
			if r.FormValue("orderId") != mockOrderID {
				http.Error(w, `{"errorCode":"6","errorMessage":"Unknown order"}`, http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, `{
				"orderId": %q,
				"OrderNumber": %d,
				"OrderStatus": "2",
				"ErrorCode": "0",
				"actionCode": "0",
				"actionCodeDescription": "Approved",
				"amount": "%d",
				"currency": "012",
				"Pan": "628058******4321",
				"cardholderName": "ALICE CITIZEN",
				"approvalCode": "APP123",
				"params": {"respCode_desc": "Approved by Issuer"}
			}`, mockOrderID, mockOrderNumber, mockAmount)

		case "/getOrderStatus.do":
			if r.FormValue("orderId") != mockOrderID {
				http.Error(w, `{"errorCode":"6","errorMessage":"Unknown order"}`, http.StatusNotFound)
				return
			}
			fmt.Fprintf(w, `{
				"orderId": %q,
				"OrderNumber": %d,
				"OrderStatus": "2",
				"ErrorCode": "0",
				"actionCode": "0",
				"amount": "%d",
				"currency": "012"
			}`, mockOrderID, mockOrderNumber, mockAmount)

		case "/refund.do":
			if r.FormValue("orderId") != mockOrderID {
				http.Error(w, `{"errorCode":"6","errorMessage":"Unknown order"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"errorCode":"0","errorMessage":"Success"}`))

		default:
			http.NotFound(w, r)
		}
	})

	ctx := t.Context()

	// Step 1: Register order
	regResp, err := client.Register(ctx, satim.RegisterOrderRequest{
		AmountMinor: mockAmount,
		OrderNumber: mockOrderNumber,
		ReturnURL:   "https://merchant.dz/checkout/success",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if regResp.OrderID != mockOrderID {
		t.Errorf("expected OrderID %s, got %s", mockOrderID, regResp.OrderID)
	}
	if regResp.OrderNumber != mockOrderNumber {
		t.Errorf("expected OrderNumber %d, got %d", mockOrderNumber, regResp.OrderNumber)
	}

	// Step 2: Confirm order after cardholder redirection
	confirmResp, err := client.Confirm(ctx, satim.ConfirmRequest{
		OrderID: regResp.OrderID,
	})
	if err != nil {
		t.Fatalf("Confirm failed: %v", err)
	}
	if !confirmResp.IsSuccessful() {
		t.Errorf("expected order to be successful")
	}
	if statusErr := confirmResp.Err(); statusErr != nil {
		t.Errorf("expected nil error from Err(), got: %v", statusErr)
	}
	if confirmResp.OrderNumber != mockOrderNumber {
		t.Errorf("expected OrderNumber %d, got %d", mockOrderNumber, confirmResp.OrderNumber)
	}
	if confirmResp.CardholderName != "ALICE CITIZEN" {
		t.Errorf("expected ALICE CITIZEN, got %s", confirmResp.CardholderName)
	}

	// Step 3: Query status idempotently
	statusResp, err := client.GetStatus(ctx, satim.GetStatusRequest{
		OrderID: regResp.OrderID,
	})
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if !statusResp.IsSuccessful() {
		t.Errorf("expected status to be successful")
	}
	if statusResp.OrderNumber != mockOrderNumber {
		t.Errorf("expected OrderNumber %d, got %d", mockOrderNumber, statusResp.OrderNumber)
	}

	// Step 4: Refund the order
	refundResp, err := client.Refund(ctx, satim.RefundRequest{
		OrderID:     regResp.OrderID,
		AmountMinor: mockAmount,
	})
	if err != nil {
		t.Fatalf("Refund failed: %v", err)
	}
	if refundResp.ErrorCode != "0" {
		t.Errorf("expected errorCode 0, got %s", refundResp.ErrorCode)
	}
}
