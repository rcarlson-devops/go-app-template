package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, srv *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)
	return rec
}

func scrape(t *testing.T, srv *Server) string {
	t.Helper()
	rec := serve(t, srv, http.MethodGet, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want %d", rec.Code, http.StatusOK)
	}
	b, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("reading /metrics body: %v", err)
	}
	return string(b)
}

func TestMetrics_CountsRequestsByRoute(t *testing.T) {
	srv, _ := testServer(t, "dev")
	serve(t, srv, http.MethodGet, "/version")

	body := scrape(t, srv)
	want := `http_requests_total{method="GET",route="/version",status="200"} 1`
	if !strings.Contains(body, want) {
		t.Errorf("/metrics does not contain %q", want)
	}
	if !strings.Contains(body, "http_request_duration_seconds_bucket") {
		t.Error("/metrics has no request duration histogram")
	}
	if !strings.Contains(body, "go_goroutines") {
		t.Error("/metrics has no Go runtime metrics")
	}
}

func TestMetrics_UnknownPathsShareOneRoute(t *testing.T) {
	srv, _ := testServer(t, "dev")
	serve(t, srv, http.MethodGet, "/scanner-a")
	serve(t, srv, http.MethodGet, "/scanner-b")

	body := scrape(t, srv)
	want := `http_requests_total{method="GET",route="/",status="200"} 2`
	if !strings.Contains(body, want) {
		t.Errorf("/metrics does not contain %q", want)
	}
	if strings.Contains(body, "scanner-a") || strings.Contains(body, "scanner-b") {
		t.Error("raw request paths leaked into metric labels")
	}
}

func TestMetrics_UnusualMethodsShareOneLabel(t *testing.T) {
	srv, _ := testServer(t, "dev")
	serve(t, srv, "FOO", "/version")

	body := scrape(t, srv)
	if strings.Contains(body, `method="FOO"`) {
		t.Error("an arbitrary method became a label value")
	}
	if !strings.Contains(body, `method="OTHER"`) {
		t.Error(`expected method="OTHER" in /metrics`)
	}
}

func TestMetrics_SkipsProbesAndScrapes(t *testing.T) {
	srv, _ := testServer(t, "dev")
	serve(t, srv, http.MethodGet, "/healthz")
	serve(t, srv, http.MethodGet, "/readyz")

	body := scrape(t, srv)
	for _, route := range []string{"/healthz", "/readyz", "/metrics", "/internal/prestop"} {
		if strings.Contains(body, `route="`+route+`"`) {
			t.Errorf("route %q should not be counted", route)
		}
	}
}
