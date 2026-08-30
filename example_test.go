package satim_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/muandane/go-satim"
)

func ExampleNewClient() {
	creds := satim.Credentials{
		Username:   os.Getenv("SATIM_USERNAME"),
		Password:   os.Getenv("SATIM_PASSWORD"),
		TerminalID: os.Getenv("SATIM_TERMINAL_ID"),
	}

	// For testing, use satim.WithTestMode(true)
	client, err := satim.NewClient(creds, satim.WithTestMode(true))
	if err != nil {
		log.Fatalf("failed to initialize client: %v", err)
	}

	_ = client
}

func ExampleClient_Register() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"orderId": "satim-order-123",
			"formUrl": "https://test.satim.dz/payment/merch/satim-order-123",
			"errorCode": "0"
		}`))
	}))
	defer server.Close()

	client, err := satim.NewClient(satim.Credentials{
		Username:   "test_user",
		Password:   "test_password",
		TerminalID: "123456",
	}, satim.WithBaseURL(server.URL), satim.WithHTTPClient(server.Client()))
	if err != nil {
		log.Fatalf("client error: %v", err)
	}

	resp, err := client.Register(context.Background(), satim.RegisterOrderRequest{
		AmountMinor: 100000, // 1000.00 DZD in minor units (centimes)
		ReturnURL:   "https://example.com/payment/return",
		FailURL:     "https://example.com/payment/fail",
		Description: "Order #54321 Payment",
		Language:    satim.LanguageFR,
	})
	if err != nil {
		log.Fatalf("registration error: %v", err)
	}

	fmt.Printf("Order ID: %s\n", resp.OrderID)
	fmt.Printf("Payment Form URL: %s\n", resp.FormURL)

	// Output:
	// Order ID: satim-order-123
	// Payment Form URL: https://test.satim.dz/payment/merch/satim-order-123
}
