package httputil

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func dummyNextHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("next response"))
}

func TestAuthorizationMiddleware(t *testing.T) {
	recorder := httptest.NewRecorder()
	testRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	authCookie := http.Cookie{
		Value: `eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJodHRwczovL3Rlc3QuZWxpb25hLmlvL2FwaSIsImV4cCI6MTc2MTg0NTY3NywiaXNzIjoiaHR0cHM6Ly90ZXN0LmVsaW9uYS5pbyIsInJvbGUiOiJhcGkiLCJyb2xlX2lkIjoiMTY5IiwidXNlcl9pZCI6ImM1M2NmYTBhLTEyMWQtNDNiNy1iZjljLTQ5ZjgzZmI3MjNlYSIsInRlbmFudF9pZCI6IjVhMDFjZDg0LWI1MTQtNDEwMS1hYzVmLTJhNmZlNTViNmI5OSIsImVudGl0bGVtZW50cyI6InVzZXIifQ.61k5wg5KdJGujGctxa2ER7WeZRCL60PlVfOJ9gprpIk`,
		Name:  "elionaAuthorization",
	}

	testRequest.AddCookie(&authCookie)

	mw := NewAuthorizationMiddleware([]byte("some-static-signing-key-for-test"), http.HandlerFunc(dummyNextHandler))
	mw.ServeHTTP(recorder, testRequest)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	if recorder.Body.String() != "next response" {
		t.Errorf("got %s, want %s", recorder.Body.String(), "next response")
	}

}
