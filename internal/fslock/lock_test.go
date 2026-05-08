package fslock

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestLockContentionHonorsContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator.lock")
	first := New(path)
	if err := first.Lock(context.Background()); err != nil {
		t.Fatalf("first Lock() error = %v", err)
	}
	defer first.Unlock()

	second := New(path)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	err := second.Lock(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Lock() error = %v, want deadline exceeded", err)
	}
}

func TestLockReleases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator.lock")
	first := New(path)
	if err := first.Lock(context.Background()); err != nil {
		t.Fatalf("first Lock() error = %v", err)
	}
	if err := first.Unlock(); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}
	second := New(path)
	if err := second.Lock(context.Background()); err != nil {
		t.Fatalf("second Lock() error = %v", err)
	}
	if err := second.Unlock(); err != nil {
		t.Fatalf("second Unlock() error = %v", err)
	}
}
