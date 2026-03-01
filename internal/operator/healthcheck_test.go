package operator

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fyltr/angee/internal/config"
)

func TestBuildProbes(t *testing.T) {
	cfg := &config.AngeeConfig{
		Name: "test",
		Services: map[string]config.ServiceSpec{
			"api": {
				Image:     "api:latest",
				Lifecycle: "platform",
				Health:    &config.HealthSpec{Path: "/healthz", Port: 3000, Interval: "15s", Timeout: "2s"},
			},
			"db": {
				Image:     "postgres",
				Lifecycle: "sidecar",
				// no health check
			},
		},
	}

	probes := buildProbes(cfg)
	if len(probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(probes))
	}
	p := probes[0]
	if p.Name != "api" {
		t.Errorf("Name = %q, want %q", p.Name, "api")
	}
	if p.URL != "http://api:3000/healthz" {
		t.Errorf("URL = %q, want %q", p.URL, "http://api:3000/healthz")
	}
	if p.Interval != 15*time.Second {
		t.Errorf("Interval = %v, want 15s", p.Interval)
	}
	if p.Timeout != 2*time.Second {
		t.Errorf("Timeout = %v, want 2s", p.Timeout)
	}
}

func TestBuildProbesDefaultPort(t *testing.T) {
	cfg := &config.AngeeConfig{
		Name: "test",
		Services: map[string]config.ServiceSpec{
			"web": {
				Image:     "web:latest",
				Lifecycle: "platform",
				Health:    &config.HealthSpec{Path: "/health"},
			},
		},
	}

	probes := buildProbes(cfg)
	if len(probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(probes))
	}
	if probes[0].URL != "http://web:8000/health" {
		t.Errorf("URL = %q, want default port 8000", probes[0].URL)
	}
}

func TestHealthCheckerProbe(t *testing.T) {
	// Start a test HTTP server that returns 200.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hc := newHealthChecker(slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := probeSpec{
		Name:     "test-svc",
		URL:      srv.URL + "/health",
		Interval: 50 * time.Millisecond,
		Timeout:  2 * time.Second,
	}

	hc.Run(ctx, []probeSpec{probe})

	// Wait for at least one probe to complete.
	time.Sleep(150 * time.Millisecond)

	status := hc.Status("test-svc")
	if status != "healthy" {
		t.Errorf("Status = %q, want %q", status, "healthy")
	}
}

func TestHealthCheckerUnhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	hc := newHealthChecker(slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := probeSpec{
		Name:     "bad-svc",
		URL:      srv.URL + "/health",
		Interval: 50 * time.Millisecond,
		Timeout:  2 * time.Second,
	}

	hc.Run(ctx, []probeSpec{probe})
	time.Sleep(150 * time.Millisecond)

	status := hc.Status("bad-svc")
	if status != "unhealthy" {
		t.Errorf("Status = %q, want %q", status, "unhealthy")
	}
}

func TestHealthCheckerNoProbe(t *testing.T) {
	hc := newHealthChecker(slog.Default())
	status := hc.Status("nonexistent")
	if status != "" {
		t.Errorf("Status = %q, want empty for unconfigured service", status)
	}
}

func TestHealthCheckerReload(t *testing.T) {
	hc := newHealthChecker(slog.Default())

	// Seed a stale result.
	hc.store("old-svc", true, "")

	cfg := &config.AngeeConfig{
		Name: "test",
		Services: map[string]config.ServiceSpec{
			"api": {
				Image:     "api:latest",
				Lifecycle: "platform",
				Health:    &config.HealthSpec{Path: "/healthz", Port: 8080},
			},
		},
	}

	probes := hc.Reload(cfg)
	if len(probes) != 1 {
		t.Fatalf("expected 1 probe, got %d", len(probes))
	}

	// The stale service should be cleaned up.
	if hc.Status("old-svc") != "" {
		t.Error("expected old-svc to be cleaned up after reload")
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		fallback time.Duration
		want     time.Duration
	}{
		{"10s", 30 * time.Second, 10 * time.Second},
		{"", 30 * time.Second, 30 * time.Second},
		{"invalid", 5 * time.Second, 5 * time.Second},
		{"1m", 30 * time.Second, 1 * time.Minute},
	}
	for _, tt := range tests {
		got := parseDuration(tt.input, tt.fallback)
		if got != tt.want {
			t.Errorf("parseDuration(%q, %v) = %v, want %v", tt.input, tt.fallback, got, tt.want)
		}
	}
}
