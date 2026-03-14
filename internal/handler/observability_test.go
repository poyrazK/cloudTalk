package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestObservabilityUsesRoutePattern(t *testing.T) {
	t.Parallel()

	r := chi.NewRouter()
	r.Use(Observability())
	r.Get("/rooms/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/rooms/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	if !strings.Contains(body, `cloudtalk_http_requests_total{method="GET",path="/rooms/{id}",status="201"}`) {
		t.Fatalf("expected route-pattern metric in body, got %q", body)
	}
	if !strings.Contains(body, `cloudtalk_http_request_duration_seconds`) {
		t.Fatalf("expected duration metric in body, got %q", body)
	}
}

func TestMetricsEndpointServesPrometheusOutput(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code: %d", rec.Code)
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, "cloudtalk_ws_connections_active") {
		t.Fatalf("expected websocket metric in body, got %q", text)
	}
	if !strings.Contains(text, "promhttp_metric_handler_requests_in_flight") {
		t.Fatalf("expected promhttp metric in body, got %q", text)
	}
}
