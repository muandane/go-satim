package satim_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muandane/go-satim"
)

var defaultTestCreds = satim.Credentials{
	Username:   "test_user",
	Password:   "test_password",
	TerminalID: "TERM01",
}

// newTestClient spins up an httptest.Server with the provided handler, registers server.Close
// via t.Cleanup, and returns a configured *satim.Client and the *httptest.Server.
func newTestClient(t *testing.T, handler http.HandlerFunc) (*satim.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := satim.NewClient(
		defaultTestCreds,
		satim.WithBaseURL(server.URL),
		satim.WithHTTPClient(server.Client()),
	)
	if err != nil {
		t.Fatalf("newTestClient failed to create client: %v", err)
	}

	return client, server
}
