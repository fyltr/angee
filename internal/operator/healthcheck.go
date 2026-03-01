package operator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fyltr/angee/internal/config"
)

// serviceHealth holds the last probe result for a service.
type serviceHealth struct {
	Healthy   bool
	LastCheck time.Time
	Error     string // empty when healthy
}

// HealthChecker performs Kubernetes-style HTTP health probes from the operator
// rather than relying on tools (curl/wget) inside the target container.
type HealthChecker struct {
	mu      sync.RWMutex
	results map[string]*serviceHealth // keyed by service name
	client  *http.Client
	log     *slog.Logger
}

// newHealthChecker creates a HealthChecker.
func newHealthChecker(logger *slog.Logger) *HealthChecker {
	return &HealthChecker{
		results: make(map[string]*serviceHealth),
		client: &http.Client{
			Timeout: 5 * time.Second,
			// Don't follow redirects — a 3xx from a health endpoint is still "up".
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		log: logger,
	}
}

// probeSpec is a single service's health probe configuration.
type probeSpec struct {
	Name     string
	URL      string
	Interval time.Duration
	Timeout  time.Duration
}

// buildProbes reads angee.yaml and returns a probeSpec for each service that
// declares a health block.
func buildProbes(cfg *config.AngeeConfig) []probeSpec {
	var probes []probeSpec
	for name, svc := range cfg.Services {
		if svc.Health == nil {
			continue
		}
		port := svc.Health.Port
		if port == 0 {
			port = 8000
		}
		interval := parseDuration(svc.Health.Interval, 30*time.Second)
		timeout := parseDuration(svc.Health.Timeout, 5*time.Second)

		probes = append(probes, probeSpec{
			Name:     name,
			URL:      fmt.Sprintf("http://%s:%d%s", name, port, svc.Health.Path),
			Interval: interval,
			Timeout:  timeout,
		})
	}
	return probes
}

// Run starts a background goroutine per service health spec. It reloads the
// config from root on each call, so newly deployed services get probed after
// the next deploy calls Reload.
func (hc *HealthChecker) Run(ctx context.Context, probes []probeSpec) {
	for _, p := range probes {
		go hc.probeLoop(ctx, p)
	}
}

// Reload stops existing probes (via context cancellation from the caller) and
// returns fresh probes from the current config.
func (hc *HealthChecker) Reload(cfg *config.AngeeConfig) []probeSpec {
	probes := buildProbes(cfg)

	// Clean up results for services that no longer declare health.
	hc.mu.Lock()
	active := make(map[string]bool, len(probes))
	for _, p := range probes {
		active[p.Name] = true
	}
	for name := range hc.results {
		if !active[name] {
			delete(hc.results, name)
		}
	}
	hc.mu.Unlock()

	return probes
}

// Status returns "healthy", "unhealthy", or "" (no health check configured).
func (hc *HealthChecker) Status(service string) string {
	hc.mu.RLock()
	defer hc.mu.RUnlock()
	h, ok := hc.results[service]
	if !ok {
		return ""
	}
	if h.Healthy {
		return "healthy"
	}
	return "unhealthy"
}

func (hc *HealthChecker) probeLoop(ctx context.Context, p probeSpec) {
	ticker := time.NewTicker(p.Interval)
	defer ticker.Stop()

	// Probe immediately on start, then on each tick.
	hc.probe(ctx, p)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hc.probe(ctx, p)
		}
	}
}

func (hc *HealthChecker) probe(ctx context.Context, p probeSpec) {
	reqCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.URL, nil)
	if err != nil {
		hc.store(p.Name, false, err.Error())
		return
	}

	resp, err := hc.client.Do(req)
	if err != nil {
		hc.store(p.Name, false, err.Error())
		return
	}
	resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400
	errMsg := ""
	if !healthy {
		errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	hc.store(p.Name, healthy, errMsg)
}

func (hc *HealthChecker) store(name string, healthy bool, errMsg string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	prev, existed := hc.results[name]
	hc.results[name] = &serviceHealth{
		Healthy:   healthy,
		LastCheck: time.Now(),
		Error:     errMsg,
	}
	// Log transitions.
	if !existed || prev.Healthy != healthy {
		if healthy {
			hc.log.Info("health check passed", "service", name)
		} else {
			hc.log.Warn("health check failed", "service", name, "error", errMsg)
		}
	}
}

// parseDuration parses a duration string, returning fallback on error or empty.
func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}
