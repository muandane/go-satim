package satim

import (
	"fmt"
	"log/slog"
)

// Language represents the supported payment page localization on SATIM.
type Language string

const (
	// LanguageFR sets the payment interface language to French (default).
	LanguageFR Language = "FR"
	// LanguageAR sets the payment interface language to Arabic.
	LanguageAR Language = "AR"
	// LanguageEN sets the payment interface language to English.
	LanguageEN Language = "EN"
)

// IsValid reports whether the language is supported by SATIM.
func (l Language) IsValid() bool {
	switch l {
	case LanguageFR, LanguageAR, LanguageEN:
		return true
	default:
		return false
	}
}

// OrderStatus represents the BPC / SATIM order state.
type OrderStatus string

const (
	// OrderStatusRegistered indicates the order was created but payment has not been completed.
	OrderStatusRegistered OrderStatus = "0"
	// OrderStatusHeld indicates funds were held for a two-phase transaction.
	OrderStatusHeld OrderStatus = "1"
	// OrderStatusApproved indicates the payment was fully authorized and successful.
	OrderStatusApproved OrderStatus = "2"
	// OrderStatusDeclined indicates authorization was reversed or declined.
	OrderStatusDeclined OrderStatus = "3"
	// OrderStatusRefunded indicates the transaction was refunded.
	OrderStatusRefunded OrderStatus = "4"
	// OrderStatusACSAuth indicates 3D-Secure ACS cardholder verification is in progress.
	OrderStatusACSAuth OrderStatus = "5"
	// OrderStatusAuthFailed indicates authorization failed.
	OrderStatusAuthFailed OrderStatus = "6"
)

// Credentials holds the API authentication details issued by SATIM via CIBWeb.dz.
type Credentials struct {
	Username   string
	Password   string
	TerminalID string
}

// Validate checks that all required credential fields are provided.
func (c Credentials) Validate() error {
	if c.Username == "" || c.Password == "" || c.TerminalID == "" {
		return ErrMissingRequiredData
	}
	return nil
}

// LogValue implements slog.LogValuer to prevent the password from appearing in structured logs.
func (c Credentials) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("username", c.Username),
		slog.String("password", "[REDACTED]"),
		slog.String("terminal_id", c.TerminalID),
	)
}

// String implements fmt.Stringer to ensure passwords are redacted when formatted with %s or %v.
func (c Credentials) String() string {
	return fmt.Sprintf("Credentials{Username: %q, Password: \"[REDACTED]\", TerminalID: %q}", c.Username, c.TerminalID)
}

// GoString implements fmt.GoStringer to ensure passwords are redacted when formatted with %#v.
func (c Credentials) GoString() string {
	return c.String()
}
