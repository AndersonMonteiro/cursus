package apierror

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteProducesTheEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()

	Write(recorder, http.StatusForbidden, CodeForbidden, "role not permitted")

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	want := `{"error":{"code":"forbidden","message":"role not permitted"}}` + "\n"
	if got := recorder.Body.String(); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
