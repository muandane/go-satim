package satim

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"net/url"
	"strconv"
)

// RegisterOrderRequest contains all parameters required to register a payment order on SATIM.
type RegisterOrderRequest struct {
	// AmountMinor is the payment amount in minor currency units (centimes).
	// Example: 1000.00 DZD must be passed as 100000.
	AmountMinor int64

	// OrderNumber is a unique 10-digit number identifying this order in SATIM.
	// If 0, a cryptographically secure random 10-digit number is automatically generated.
	OrderNumber int64

	// ReturnURL is the URL where the cardholder is redirected after successful authorization.
	ReturnURL string

	// FailURL is the URL where the cardholder is redirected if authorization fails.
	// If empty, FailURL defaults to ReturnURL.
	FailURL string

	// Description is an optional human-readable description of the transaction (max 598 characters).
	Description string

	// Language specifies the payment page localization (FR, AR, or EN). Defaults to FR.
	Language Language

	// SessionTimeoutSecs specifies the payment session timeout in seconds (600 to 86400).
	SessionTimeoutSecs int

	// UserDefinedFields contains custom metadata attached to the order.
	UserDefinedFields map[string]string
}

// RegisterOrderResponse contains the response data from the /register.do endpoint.
type RegisterOrderResponse struct {
	// OrderID is SATIM's unique UUID/identifier for this registered payment.
	OrderID string `json:"orderId"`

	// OrderNumber is the 10-digit unique identifier assigned to this order.
	OrderNumber int64 `json:"orderNumber,omitempty"`

	// FormURL is the hosted SATIM payment page URL where the customer must be redirected.
	FormURL string `json:"formUrl"`

	// ErrorCode indicates the result code (0 for success).
	ErrorCode string `json:"errorCode"`

	// ErrorMessage contains error details if the registration failed.
	ErrorMessage string `json:"errorMessage"`

	// Raw contains the unparsed JSON response map.
	Raw map[string]any `json:"-"`
}

func (r *RegisterOrderResponse) setRaw(raw map[string]any) {
	r.Raw = raw
}

// Validate checks request validity and sets required defaults.
func (r *RegisterOrderRequest) Validate() error {
	if r.AmountMinor <= 0 {
		return ErrInvalidAmount
	}

	if r.ReturnURL == "" {
		return fmt.Errorf("%w: ReturnURL is required", ErrMissingRequiredData)
	}

	if _, err := url.ParseRequestURI(r.ReturnURL); err != nil {
		return fmt.Errorf("%w: ReturnURL: %w", ErrInvalidURL, err)
	}

	r.FailURL = cmp.Or(r.FailURL, r.ReturnURL)
	if _, err := url.ParseRequestURI(r.FailURL); err != nil {
		return fmt.Errorf("%w: FailURL: %w", ErrInvalidURL, err)
	}

	if r.OrderNumber != 0 {
		if r.OrderNumber < 1000000000 || r.OrderNumber > 9999999999 {
			return ErrInvalidOrderNumber
		}
	} else {
		generated, err := GenerateOrderNumber()
		if err != nil {
			return fmt.Errorf("satim: generate order number: %w", err)
		}
		r.OrderNumber = generated
	}

	r.Language = cmp.Or(r.Language, LanguageFR)
	if !r.Language.IsValid() {
		return ErrInvalidLanguage
	}

	if r.Description != "" && len([]rune(r.Description)) > 598 {
		return ErrDescriptionTooLong
	}

	if r.SessionTimeoutSecs != 0 {
		if r.SessionTimeoutSecs < 600 || r.SessionTimeoutSecs > 86400 {
			return ErrInvalidTimeout
		}
	}

	return nil
}

// GenerateOrderNumber produces a cryptographically secure random 10-digit number.
func GenerateOrderNumber() (int64, error) {
	// Range: 1000000000 to 9999999999 (width: 9000000000)
	maxVal := big.NewInt(9000000000)
	n, err := rand.Int(rand.Reader, maxVal)
	if err != nil {
		return 0, err
	}
	return n.Int64() + 1000000000, nil
}

// Register registers a new payment order with the SATIM gateway and returns the payment URL.
//
// SECURITY WARNING:
// When the cardholder completes the payment and is redirected back to ReturnURL or FailURL,
// query parameters in the redirect URL must NEVER be trusted as proof of payment.
// Callers MUST invoke Confirm or GetStatus server-side to independently verify the transaction state.
//
// FAILURE RECOVERY:
// If Register returns a network/transport error with an unknown outcome, do not blindly retry
// Register with a new order number. Use GetStatus to verify if the order was created, or check
// your system's reconciliation queue.
func (c *Client) Register(ctx context.Context, req RegisterOrderRequest) (*RegisterOrderResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	additionalData := make(map[string]string)
	additionalData["force_terminal_id"] = c.creds.TerminalID

	maps.Copy(additionalData, req.UserDefinedFields)

	jsonParamsBytes, err := json.Marshal(additionalData)
	if err != nil {
		return nil, fmt.Errorf("satim: serialize jsonParams: %w", err)
	}

	form := make(url.Values)
	form.Set("orderNumber", strconv.FormatInt(req.OrderNumber, 10))
	form.Set("amount", strconv.FormatInt(req.AmountMinor, 10))
	form.Set("currency", CurrencyDZD) // SATIM is strictly DZD
	form.Set("returnUrl", req.ReturnURL)
	form.Set("failUrl", req.FailURL)
	form.Set("language", string(req.Language))
	form.Set("jsonParams", string(jsonParamsBytes))

	if req.Description != "" {
		form.Set("description", req.Description)
	}

	if req.SessionTimeoutSecs > 0 {
		form.Set("sessionTimeoutSecs", strconv.Itoa(req.SessionTimeoutSecs))
	}

	resp, err := c.execute[RegisterOrderResponse](ctx, "/register.do", form, false)
	if err != nil {
		return nil, err
	}

	resp.OrderNumber = req.OrderNumber
	return resp, nil
}
