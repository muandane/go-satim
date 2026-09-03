package satim

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ActionCode represents the BPC transaction response code.
type ActionCode string

const (
	// ActionCodeApproved indicates payment was authorized.
	ActionCodeApproved ActionCode = "0"
	// ActionCodeCancelled indicates the payment was aborted by the cardholder or system.
	ActionCodeCancelled ActionCode = "10"
	// ActionCodeDeclined indicates authorization was declined.
	ActionCodeDeclined ActionCode = "2003"
	// ActionCodeSessionExpired indicates the payment page session timed out.
	ActionCodeSessionExpired ActionCode = "-2007"
)

// ConfirmRequest specifies the order to confirm upon cardholder return.
type ConfirmRequest struct {
	// OrderID is the unique SATIM transaction identifier.
	OrderID string

	// Language specifies the error/response localization. Defaults to FR.
	Language Language
}

// Validate checks whether the confirmation parameters are valid.
func (r *ConfirmRequest) Validate() error {
	if r.OrderID == "" {
		return fmt.Errorf("%w: OrderID is required", ErrMissingRequiredData)
	}
	r.Language = cmp.Or(r.Language, LanguageFR)
	if !r.Language.IsValid() {
		return ErrInvalidLanguage
	}
	return nil
}

// GetStatusRequest specifies the order whose status to query.
type GetStatusRequest struct {
	// OrderID is the unique SATIM transaction identifier.
	OrderID string

	// Language specifies the error/response localization. Defaults to FR.
	Language Language
}

// Validate checks whether the status query parameters are valid.
func (r *GetStatusRequest) Validate() error {
	if r.OrderID == "" {
		return fmt.Errorf("%w: OrderID is required", ErrMissingRequiredData)
	}
	r.Language = cmp.Or(r.Language, LanguageFR)
	if !r.Language.IsValid() {
		return ErrInvalidLanguage
	}
	return nil
}

// OrderStatusResponse contains the detailed transaction state returned by Confirm or GetStatus.
type OrderStatusResponse struct {
	// OrderID is the unique SATIM transaction identifier.
	OrderID string `json:"orderId"`

	// OrderNumber is the 10-digit merchant order number assigned during registration.
	OrderNumber int64 `json:"OrderNumber,omitempty"`

	// OrderStatus represents the BPC order state ("0" through "6").
	OrderStatus OrderStatus `json:"OrderStatus"`

	// ErrorCode indicates the API error code (0 for success).
	ErrorCode string `json:"ErrorCode"`

	// ErrorMessage contains error details if the query failed.
	ErrorMessageText string `json:"ErrorMessage"`

	// ActionCode is BPC's ISO response / action code (0 for approved, 10 cancelled, -2007 expired, 2003 declined).
	ActionCode string `json:"actionCode"`

	// ActionCodeDescription is the human-readable description of the action code.
	ActionCodeDescription string `json:"actionCodeDescription"`

	// AmountMinor is the transaction amount in minor currency units (centimes).
	AmountMinor int64 `json:"amount"`

	// Currency is the currency numeric code ("012" for DZD).
	Currency string `json:"currency"`

	// Pan is the masked payment card number (e.g. 628058******1234).
	Pan string `json:"Pan"`

	// CardholderName is the cardholder's name as printed on the card.
	CardholderName string `json:"cardholderName"`

	// Expiration is the card expiration date (e.g. 202612).
	Expiration string `json:"expiration"`

	// ApprovalCode is the bank authorization code for successful transactions.
	ApprovalCode string `json:"approvalCode"`

	// IP is the IP address of the cardholder.
	IP string `json:"Ip"`

	// Params contains extra key-value parameters returned by SATIM (e.g. respCode, respCode_desc).
	Params map[string]string `json:"params"`

	// Raw contains the unparsed JSON response map.
	Raw map[string]any `json:"-"`
}

func (r *OrderStatusResponse) setRaw(raw map[string]any) {
	r.Raw = raw
}

// UnmarshalJSON handles flexible numeric and string types in BPC JSON responses.
func (r *OrderStatusResponse) UnmarshalJSON(data []byte) error {
	type Alias OrderStatusResponse
	aux := struct {
		*Alias
		RawOrderNumber      any `json:"OrderNumber"`
		RawOrderNumberLower any `json:"orderNumber"`
		RawOrderStatus      any `json:"OrderStatus"`
		RawErrorCode        any `json:"ErrorCode"`
		RawActionCode       any `json:"actionCode"`
		RawAmount           any `json:"amount"`
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if aux.RawOrderNumber != nil {
		if val, ok := parseNumericInt64(aux.RawOrderNumber); ok {
			r.OrderNumber = val
		}
	} else if aux.RawOrderNumberLower != nil {
		if val, ok := parseNumericInt64(aux.RawOrderNumberLower); ok {
			r.OrderNumber = val
		}
	}
	if aux.RawOrderStatus != nil {
		r.OrderStatus = OrderStatus(fmt.Sprint(aux.RawOrderStatus))
	}
	if aux.RawErrorCode != nil {
		r.ErrorCode = fmt.Sprint(aux.RawErrorCode)
	}
	if aux.RawActionCode != nil {
		r.ActionCode = fmt.Sprint(aux.RawActionCode)
	}
	if aux.RawAmount != nil {
		if val, ok := parseNumericInt64(aux.RawAmount); ok {
			r.AmountMinor = val
		}
	}

	return nil
}

func parseNumericInt64(val any) (int64, bool) {
	switch v := val.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		v = strings.TrimSpace(v)
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n, true
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return int64(f), true
		}
	}
	return 0, false
}

// IsSuccessful reports whether the payment was successfully authorized and paid.
func (r *OrderStatusResponse) IsSuccessful() bool {
	return r.OrderStatus == OrderStatusApproved
}

// IsPending reports whether the order is created or undergoing 3D-Secure authentication and awaiting completion.
func (r *OrderStatusResponse) IsPending() bool {
	return r.OrderStatus == OrderStatusRegistered || r.OrderStatus == OrderStatusACSAuth
}

// IsRejected reports whether the transaction was declined or rejected.
func (r *OrderStatusResponse) IsRejected() bool {
	if r.OrderStatus == OrderStatusApproved || r.OrderStatus == OrderStatusRefunded || r.OrderStatus == OrderStatusRegistered {
		return false
	}
	if r.ActionCode == string(ActionCodeDeclined) || r.OrderStatus == OrderStatusDeclined || r.OrderStatus == OrderStatusAuthFailed {
		return true
	}
	return false
}

// IsRefunded reports whether the transaction was refunded.
func (r *OrderStatusResponse) IsRefunded() bool {
	return r.OrderStatus == OrderStatusRefunded
}

// IsCancelled reports whether the customer or gateway cancelled the transaction.
func (r *OrderStatusResponse) IsCancelled() bool {
	if r.OrderStatus == OrderStatusApproved || r.OrderStatus == OrderStatusRefunded {
		return false
	}
	return r.ActionCode == string(ActionCodeCancelled)
}

// IsExpired reports whether the payment session timed out.
func (r *OrderStatusResponse) IsExpired() bool {
	return r.ActionCode == string(ActionCodeSessionExpired)
}

// IsFailed reports whether the transaction failed and was not refunded or pending.
func (r *OrderStatusResponse) IsFailed() bool {
	return !r.IsSuccessful() && !r.IsRefunded() && !r.IsPending()
}

// Err returns a sentinel error corresponding to the transaction outcome, or nil if the payment was successful or pending.
func (r *OrderStatusResponse) Err() error {
	switch {
	case r.IsSuccessful(), r.IsPending():
		return nil
	case r.IsCancelled():
		return ErrPaymentCancelled
	case r.IsExpired():
		return ErrSessionExpired
	case r.IsRejected():
		return ErrPaymentDeclined
	default:
		if r.ErrorMessageText != "" {
			return errors.New(r.ErrorMessageText)
		}
		return errors.New("satim: payment not completed")
	}
}

// SuccessMessage returns the success description or a localized default.
func (r *OrderStatusResponse) SuccessMessage() string {
	if !r.IsSuccessful() {
		return r.ErrorMessage()
	}
	if desc, ok := r.Params["respCode_desc"]; ok && desc != "" {
		return desc
	}
	if r.ActionCodeDescription != "" {
		return r.ActionCodeDescription
	}
	return "Payment was successful"
}

// ErrorMessage returns the error description or a localized default.
func (r *OrderStatusResponse) ErrorMessage() string {
	if r.IsRejected() {
		return "« Votre transaction a été rejetée/ Your transaction was rejected/ تم رفض معاملتك »"
	}
	if r.IsRefunded() {
		return "Payment was refunded"
	}
	if desc, ok := r.Params["respCode_desc"]; ok && desc != "" {
		return desc
	}
	if r.ActionCodeDescription != "" {
		return r.ActionCodeDescription
	}
	if r.ErrorMessageText != "" {
		return r.ErrorMessageText
	}
	return "Payment failed"
}

// MaskedPAN returns the masked payment card number.
func (r *OrderStatusResponse) MaskedPAN() string {
	return r.Pan
}

// Confirm finalizes / completes the payment order after cardholder redirection.
//
// BPC SEMANTICS:
// In the BPC SmartVista platform, /confirmOrder.do performs order completion after the cardholder
// returns from the gateway. It transition the transaction to the confirmed state.
func (c *Client) Confirm(ctx context.Context, req ConfirmRequest) (*OrderStatusResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	form := make(url.Values)
	form.Set("orderId", req.OrderID)
	form.Set("language", string(req.Language))

	return c.execute[OrderStatusResponse](ctx, "/confirmOrder.do", form, false)
}

// GetStatus queries the current state of an order idempotently without modifying state.
//
// BPC SEMANTICS:
// /getOrderStatus.do is a read-only endpoint that can be queried at any time for status polling,
// reconciliation, and failure recovery. Transient network errors on GetStatus are safely retried.
func (c *Client) GetStatus(ctx context.Context, req GetStatusRequest) (*OrderStatusResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	form := make(url.Values)
	form.Set("orderId", req.OrderID)
	form.Set("language", string(req.Language))

	return c.execute[OrderStatusResponse](ctx, "/getOrderStatus.do", form, true)
}
