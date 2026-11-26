package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesSPAFallback(t *testing.T) {
	h := Handler()
	for _, path := range []string{"/", "/hosts", "/flows/123"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d", path, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "frontend not built") && !strings.Contains(body, `id="root"`) {
			t.Fatalf("%s: unexpected body %q", path, body)
		}
	}
}

func TestHandlerRejectsAPIPaths(t *testing.T) {
	h := Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
