package satim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ConfirmRequest specifies the order to confirm upon cardholder return.
type ConfirmRequest struct {
	// OrderID is the unique SATIM transaction identifier.
	OrderID string

	// Language specifies the error/response localization. Defaults to FR.
	Language Language
}

// GetStatusRequest specifies the order whose status to query.
type GetStatusRequest struct {
	// OrderID is the unique SATIM transaction identifier.
	OrderID string

	// Language specifies the error/response localization. Defaults to FR.
	Language Language
}

// OrderStatusResponse contains the detailed transaction state returned by Confirm or GetStatus.
type OrderStatusResponse struct {
	// OrderID is the unique SATIM transaction identifier.
	OrderID string `json:"orderId"`

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

// UnmarshalJSON handles flexible numeric and string types in BPC JSON responses.
func (r *OrderStatusResponse) UnmarshalJSON(data []byte) error {
	type Alias OrderStatusResponse
	aux := struct {
		*Alias
		RawOrderStatus any `json:"OrderStatus"`
		RawErrorCode   any `json:"ErrorCode"`
		RawActionCode  any `json:"actionCode"`
		RawAmount      any `json:"amount"`
	}{
		Alias: (*Alias)(r),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
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
		if val, err := strconv.ParseInt(fmt.Sprint(aux.RawAmount), 10, 64); err == nil {
			r.AmountMinor = val
		}
	}

	return nil
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
	if r.ActionCode == "2003" || r.OrderStatus == OrderStatusDeclined || r.OrderStatus == OrderStatusAuthFailed {
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
	return r.ActionCode == "10"
}

// IsExpired reports whether the payment session timed out.
func (r *OrderStatusResponse) IsExpired() bool {
	return r.ActionCode == "-2007"
}

// IsFailed reports whether the transaction failed and was not refunded or pending.
func (r *OrderStatusResponse) IsFailed() bool {
	return !r.IsSuccessful() && !r.IsRefunded() && !r.IsPending()
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
	if req.OrderID == "" {
		return nil, fmt.Errorf("%w: OrderID is required", ErrMissingRequiredData)
	}

	if req.Language == "" {
		req.Language = LanguageFR
	} else if !req.Language.IsValid() {
		return nil, ErrInvalidLanguage
	}

	form := make(url.Values)
	form.Set("orderId", req.OrderID)
	form.Set("language", string(req.Language))

	body, err := c.doRequest(ctx, "/confirmOrder.do", form, false)
	if err != nil {
		return nil, err
	}

	var resp OrderStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("satim: parse confirm response: %w", err)
	}

	_ = json.Unmarshal(body, &resp.Raw)

	return &resp, nil
}

// GetStatus queries the current state of an order idempotently without modifying state.
//
// BPC SEMANTICS:
// /getOrderStatus.do is a read-only endpoint that can be queried at any time for status polling,
// reconciliation, and failure recovery. Transient network errors on GetStatus are safely retried.
func (c *Client) GetStatus(ctx context.Context, req GetStatusRequest) (*OrderStatusResponse, error) {
	if req.OrderID == "" {
		return nil, fmt.Errorf("%w: OrderID is required", ErrMissingRequiredData)
	}

	if req.Language == "" {
		req.Language = LanguageFR
	} else if !req.Language.IsValid() {
		return nil, ErrInvalidLanguage
	}

	form := make(url.Values)
	form.Set("orderId", req.OrderID)
	form.Set("language", string(req.Language))

	body, err := c.doRequest(ctx, "/getOrderStatus.do", form, true)
	if err != nil {
		return nil, err
	}

	var resp OrderStatusResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("satim: parse getOrderStatus response: %w", err)
	}

	_ = json.Unmarshal(body, &resp.Raw)

	return &resp, nil
}
