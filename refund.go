package satim

import (
	"context"
	"net/url"
	"strconv"
)

// RefundRequest contains the parameters to issue a partial or full refund for an order.
type RefundRequest struct {
	// OrderID is the unique SATIM transaction identifier to refund.
	OrderID string

	// AmountMinor is the refund amount in minor currency units (centimes).
	AmountMinor int64

	// Language specifies the error/response localization. Defaults to FR.
	Language Language
}

// RefundResponse contains the result from the /refund.do endpoint.
type RefundResponse struct {
	// ErrorCode indicates the API error code (0 for success).
	ErrorCode string `json:"errorCode"`

	// ErrorMessage contains error details if the refund failed.
	ErrorMessage string `json:"errorMessage"`

	// Raw contains the unparsed JSON response map.
	Raw map[string]any `json:"-"`
}

func (r *RefundResponse) setRaw(raw map[string]any) {
	r.Raw = raw
}

// Validate verifies refund request parameters without mutating the receiver.
func (r *RefundRequest) Validate() error {
	if err := validateOrderIDAndLanguage(r.OrderID, &r.Language); err != nil {
		return err
	}
	if r.AmountMinor <= 0 {
		return ErrInvalidAmount
	}
	return nil
}

// Refund issues a partial or full refund for an authorized order on the SATIM gateway.
//
// FAILURE RECOVERY:
// If Refund fails due to a network or transport error with an unknown outcome, do not
// re-execute Refund blindly. Verify the transaction status using GetStatus(orderID) to
// check whether OrderStatus is OrderStatusRefunded ("4").
func (c *Client) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	form := make(url.Values)
	form.Set("orderId", req.OrderID)
	form.Set("amount", strconv.FormatInt(req.AmountMinor, 10))
	form.Set("language", string(effectiveLanguage(req.Language)))

	return c.execute[RefundResponse](ctx, "/refund.do", form, false)
}
