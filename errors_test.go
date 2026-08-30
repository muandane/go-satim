package satim_test

import (
	"errors"
	"testing"

	"github.com/muandane/go-satim"
)

func TestAPIError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      *satim.APIError
		expected string
	}{
		{
			name: "with error message",
			err: &satim.APIError{
				ErrorCode:    "5",
				ErrorMessage: "Invalid credentials",
				HTTPStatus:   401,
			},
			expected: `satim api error: code=5, message="Invalid credentials" (HTTP 401)`,
		},
		{
			name: "without error message",
			err: &satim.APIError{
				ErrorCode:  "7",
				HTTPStatus: 500,
			},
			expected: `satim api error: code=7 (HTTP 500)`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.expected {
				t.Errorf("Error() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestAPIError_Is(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		errCode   string
		targetErr error
		want      bool
	}{
		{name: "code 5 matches ErrInvalidCredentials", errCode: "5", targetErr: satim.ErrInvalidCredentials, want: true},
		{name: "code 6 matches ErrOrderNotFound", errCode: "6", targetErr: satim.ErrOrderNotFound, want: true},
		{name: "code 1 matches ErrOrderAlreadyExists", errCode: "1", targetErr: satim.ErrOrderAlreadyExists, want: true},
		{name: "code 7 matches ErrSystemError", errCode: "7", targetErr: satim.ErrSystemError, want: true},
		{name: "code 2 matches ErrPaymentDeclined", errCode: "2", targetErr: satim.ErrPaymentDeclined, want: true},
		{name: "code 3 matches ErrPaymentDeclined", errCode: "3", targetErr: satim.ErrPaymentDeclined, want: true},
		{name: "unmatched code returns false", errCode: "99", targetErr: satim.ErrSystemError, want: false},
		{name: "unmatched target returns false", errCode: "5", targetErr: satim.ErrMissingRequiredData, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := &satim.APIError{ErrorCode: tc.errCode}
			if got := errors.Is(err, tc.targetErr); got != tc.want {
				t.Errorf("errors.Is(code=%s, %v) = %v, want %v", tc.errCode, tc.targetErr, got, tc.want)
			}
		})
	}
}
