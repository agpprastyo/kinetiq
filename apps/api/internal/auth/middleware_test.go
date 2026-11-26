package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientRejectsLegacyTestCookies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "secret")
	for _, cookie := range []string{"admin_session", "ops_session_x", "user1_session_cookie", "user2_session_cookie"} {
		res, err := c.ValidateSession(cookie)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", cookie, err)
		}
		if res.Authenticated {
			t.Errorf("%s: must not authenticate without auth service approval", cookie)
		}
	}
}
