# go-satim

<p align="center">
  <img src="images/satim.png" alt="SATIM Logo" width="260" />
</p>

[![Go Reference](https://pkg.go.dev/badge/github.com/muandane/go-satim.svg)](https://pkg.go.dev/github.com/muandane/go-satim)
[![CI](https://github.com/muandane/go-satim/actions/workflows/ci.yml/badge.svg)](https://github.com/muandane/go-satim/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/muandane/go-satim)](https://goreportcard.com/report/github.com/muandane/go-satim)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE.md)

A production-grade Go client for **Satim.dz**, the national interbank payment gateway in Algeria.

This library supports payment registration, confirmation, status querying, and refunds for **CIB** (Carte Interbancaire) and **Edahabia** cards via SATIM's BPC SmartVista REST API.

---

## Requirements

- Go 1.27 or higher
- SATIM merchant credentials (`username`, `password`, `terminal_id`) issued via [CIBWeb.dz](https://www.cibweb.dz/)

---

## Installation

```bash
go get github.com/muandane/go-satim
```

---

## Security Notice: Payment Verification

> [!WARNING]
> **Never trust URL query parameters as proof of payment.**
> When a customer is redirected back to your `returnUrl` or `failUrl`, client-side query parameters can be manipulated. You must always make a server-side call to `client.Confirm(...)` or `client.GetStatus(...)` using the `orderId` to verify the payment state before fulfilling an order or granting access.

---

## Usage

### 1. Initialize the Client

```go
package main

import (
	"log"
	"os"

	"github.com/muandane/go-satim"
)

func main() {
	creds := satim.Credentials{
		Username:   os.Getenv("SATIM_USERNAME"),
		Password:   os.Getenv("SATIM_PASSWORD"),
		TerminalID: os.Getenv("SATIM_TERMINAL_ID"),
	}

	// Use satim.WithTestMode(true) for test.satim.dz sandbox
	client, err := satim.NewClient(creds, satim.WithTestMode(true))
	if err != nil {
		log.Fatalf("failed to initialize SATIM client: %v", err)
	}

	_ = client
}
```

---

### 2. Register a Payment Order

Amounts are specified strictly in **minor currency units** (`AmountMinor int64`, centimes):

- 1000.00 DZD = `100000`

```go
ctx := context.Background()

req := satim.RegisterOrderRequest{
	AmountMinor: 100000, // 1000.00 DZD
	ReturnURL:   "https://your-domain.dz/payment/callback",
	FailURL:     "https://your-domain.dz/payment/callback", // Optional: defaults to ReturnURL
	Description: "Order #987654",
	Language:    satim.LanguageFR, // LanguageFR, LanguageAR, or LanguageEN
	UserDefinedFields: map[string]string{
		"customer_id": "cust_12345",
	},
}

resp, err := client.Register(ctx, req)
if err != nil {
	log.Fatalf("payment registration failed: %v", err)
}

// Redirect customer to the SATIM hosted payment page
http.Redirect(w, r, resp.FormURL, http.StatusFound)
```

---

### 3. Verify Payment on Customer Return

When the cardholder completes or cancels payment, SATIM redirects them to your callback URL with an `orderId` parameter.

```go
orderID := r.URL.Query().Get("orderId")

statusResp, err := client.Confirm(ctx, satim.ConfirmRequest{
	OrderID:  orderID,
	Language: satim.LanguageFR,
})
if err != nil {
	log.Fatalf("payment confirmation failed: %v", err)
}

if statusResp.IsSuccessful() {
	log.Printf("Payment confirmed! Cardholder: %s, Masked PAN: %s, Approval Code: %s",
		statusResp.CardholderName, statusResp.MaskedPAN(), statusResp.ApprovalCode)
} else if statusResp.IsRejected() {
	log.Printf("Payment rejected: %s", statusResp.ErrorMessage())
} else if statusResp.IsCancelled() {
	log.Println("Payment cancelled by cardholder.")
}
```

---

### 4. Query Order Status (Read-Only)

Use `GetStatus` to check transaction state at any point (e.g. cron reconciliation or polling):

```go
statusResp, err := client.GetStatus(ctx, satim.GetStatusRequest{
	OrderID:  orderID,
	Language: satim.LanguageFR,
})
if err != nil {
	log.Fatalf("status check failed: %v", err)
}

log.Printf("Order Status: %s, Paid: %t", statusResp.OrderStatus, statusResp.IsSuccessful())
```

---

### 5. Issue a Refund

```go
refundResp, err := client.Refund(ctx, satim.RefundRequest{
	OrderID:     orderID,
	AmountMinor: 50000, // 500.00 DZD
	Language:    satim.LanguageFR,
})
if err != nil {
	log.Fatalf("refund failed: %v", err)
}

log.Println("Refund processed successfully")
```

---

## Error Handling

All gateway error codes are mapped to typed sentinel errors compatible with `errors.Is`:

```go
resp, err := client.Register(ctx, req)
if err != nil {
	switch {
	case errors.Is(err, satim.ErrInvalidCredentials):
		log.Println("Authentication error: check username, password, or terminal ID")
	case errors.Is(err, satim.ErrOrderAlreadyExists):
		log.Println("Duplicate order number: generate a new order number")
	case errors.Is(err, satim.ErrSystemError):
		log.Println("SATIM system error: retry later")
	default:
		log.Printf("SATIM error: %v", err)
	}
}
```

---

## Failure Recovery

If a network or transport error occurs during `Register`, `Confirm`, or `Refund`:

1. **Do not automatically retry mutation operations** without verifying their state. Retrying a refund could issue a duplicate refund.
2. **Query `GetStatus(orderID)`** to check if the transaction was recorded and what state it reached.
3. If `Register` failed before receiving an `orderId`, check your internal database or generate a new unique 10-digit `OrderNumber`.

---

## Testing

Run unit tests locally with race detection:

```bash
go test -v -race ./...
```

Tests run against an internal `httptest.Server` mock that simulates BPC REST API behavior.

---

## License

This project is licensed under the [MIT License](LICENSE.md).
