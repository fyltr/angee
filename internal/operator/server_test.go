package operator

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ── Auth middleware tests ────────────────────────────────────────────────────

func TestAuthMiddlewareNoKey(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(inner, "", testLogger())
	req := httptest.NewRequest("GET", "/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected inner handler to be called when no key configured")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthMiddlewareValid(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(inner, "test-secret", testLogger())
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "Bearer test-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected inner handler to be called with valid token")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestAuthMiddlewareInvalid(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := authMiddleware(inner, "test-secret", testLogger())
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("inner handler should NOT be called with invalid token")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddlewareNoHeader(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := authMiddleware(inner, "test-secret", testLogger())
	req := httptest.NewRequest("GET", "/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("inner handler should NOT be called without auth header")
	}
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestAuthMiddlewareHealthBypass(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := authMiddleware(inner, "test-secret", testLogger())
	req := httptest.NewRequest("GET", "/health", nil)
	// No Authorization header
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("expected /health to bypass auth")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ── Recovery middleware tests ────────────────────────────────────────────────

func TestRecoveryMiddleware(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := recoveryMiddleware(inner, testLogger())
	req := httptest.NewRequest("GET", "/status", nil)
	rr := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}

func TestRecoveryMiddlewareNoPanic(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := recoveryMiddleware(inner, testLogger())
	req := httptest.NewRequest("GET", "/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ── CORS middleware tests ────────────────────────────────────────────────────

func TestCORSAllowedOrigin(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(inner, []string{"http://localhost:*"})
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:3000" {
		t.Errorf("Allow-Origin = %q, want %q", origin, "http://localhost:3000")
	}
}

func TestCORSExactOrigin(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(inner, []string{"https://example.com"})
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "https://example.com" {
		t.Errorf("Allow-Origin = %q, want %q", origin, "https://example.com")
	}
}

func TestCORSBlockedOrigin(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(inner, []string{"https://example.com"})
	req := httptest.NewRequest("GET", "/status", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "" {
		t.Errorf("Allow-Origin = %q, want empty for blocked origin", origin)
	}
}

func TestCORSPreflight(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := corsMiddleware(inner, []string{"http://localhost:*"})
	req := httptest.NewRequest("OPTIONS", "/deploy", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if called {
		t.Error("inner handler should not be called for OPTIONS preflight")
	}
	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
}

// ── matchOrigin tests ────────────────────────────────────────────────────────

func TestMatchOrigin(t *testing.T) {
	tests := []struct {
		origin   string
		patterns []string
		want     bool
	}{
		{"http://localhost:3000", []string{"http://localhost:*"}, true},
		{"http://localhost:9000", []string{"http://localhost:*"}, true},
		{"https://example.com", []string{"https://example.com"}, true},
		{"https://evil.com", []string{"https://example.com"}, false},
		{"http://other:3000", []string{"http://localhost:*"}, false},
		{"http://localhost:3000", []string{}, false},
	}

	for _, tt := range tests {
		got := matchOrigin(tt.origin, tt.patterns)
		if got != tt.want {
			t.Errorf("matchOrigin(%q, %v) = %v, want %v", tt.origin, tt.patterns, got, tt.want)
		}
	}
}
