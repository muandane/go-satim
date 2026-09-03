package satim

import (
	"errors"
	"fmt"
)

// Sentinel validation and API errors.
var (
	// ErrMissingRequiredData indicates required parameters (e.g. credentials, return URL) were omitted.
	ErrMissingRequiredData = errors.New("satim: missing required data")

	// ErrInvalidCredentials indicates access was denied by SATIM (BPC error code 5).
	ErrInvalidCredentials = errors.New("satim: invalid credentials or terminal ID (BPC error 5)")

	// ErrOrderNotFound indicates the requested order ID does not exist in SATIM (BPC error code 6).
	ErrOrderNotFound = errors.New("satim: unknown order ID (BPC error 6)")

	// ErrOrderAlreadyExists indicates an order with the same order number was already registered (BPC error code 1).
	ErrOrderAlreadyExists = errors.New("satim: order number already registered (BPC error 1)")

	// ErrPaymentDeclined indicates the card issuer or SATIM rejected the payment.
	ErrPaymentDeclined = errors.New("satim: payment declined")

	// ErrPaymentCancelled indicates the payment was aborted by the cardholder or system (actionCode 10).
	ErrPaymentCancelled = errors.New("satim: payment cancelled (actionCode 10)")

	// ErrSessionExpired indicates the payment page session timed out (actionCode -2007).
	ErrSessionExpired = errors.New("satim: payment session expired (actionCode -2007)")

	// ErrSystemError indicates a system failure on SATIM's end (BPC error code 7).
	ErrSystemError = errors.New("satim: gateway system error (BPC error 7)")

	// ErrInvalidAmount indicates the payment amount is zero or negative.
	ErrInvalidAmount = errors.New("satim: amount must be a positive integer in minor units (centimes)")

	// ErrInvalidOrderNumber indicates the order number does not match SATIM's 10-digit requirement.
	ErrInvalidOrderNumber = errors.New("satim: order number must be exactly 10 digits")

	// ErrInvalidTimeout indicates the session timeout is outside the allowed range (600 to 86400 seconds).
	ErrInvalidTimeout = errors.New("satim: session timeout must be between 600 and 86400 seconds")

	// ErrInvalidURL indicates the return or fail URL is malformed.
	ErrInvalidURL = errors.New("satim: return or fail URL is invalid")

	// ErrInvalidLanguage indicates an unsupported language code was specified.
	ErrInvalidLanguage = errors.New("satim: unsupported language; allowed values are FR, AR, EN")

	// ErrDescriptionTooLong indicates the order description exceeds 598 characters.
	ErrDescriptionTooLong = errors.New("satim: description exceeds maximum length of 598 characters")

	// ErrInvalidUUID indicates an order ID is not a valid UUID format.
	ErrInvalidUUID = errors.New("satim: invalid UUID format")
)

// APIError represents an error response returned by the SATIM/BPC REST gateway.
type APIError struct {
	ErrorCode    string         `json:"errorCode"`
	ErrorMessage string         `json:"errorMessage"`
	HTTPStatus   int            `json:"-"`
	Raw          map[string]any `json:"-"`
}

func (e *APIError) Error() string {
	if e.ErrorMessage != "" {
		return fmt.Sprintf("satim api error: code=%s, message=%q (HTTP %d)", e.ErrorCode, e.ErrorMessage, e.HTTPStatus)
	}
	return fmt.Sprintf("satim api error: code=%s (HTTP %d)", e.ErrorCode, e.HTTPStatus)
}

// Is reports whether e matches target for errors.Is checks.
func (e *APIError) Is(target error) bool {
	switch {
	case target == ErrInvalidCredentials && e.ErrorCode == "5":
		return true
	case target == ErrOrderNotFound && e.ErrorCode == "6":
		return true
	case target == ErrOrderAlreadyExists && e.ErrorCode == "1":
		return true
	case target == ErrSystemError && e.ErrorCode == "7":
		return true
	case target == ErrPaymentDeclined && (e.ErrorCode == "2" || e.ErrorCode == "3"):
		return true
	case target == ErrPaymentCancelled && e.ErrorCode == "10":
		return true
	case target == ErrSessionExpired && e.ErrorCode == "-2007":
		return true
	default:
		return false
	}
}
