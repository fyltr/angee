package fslock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type Lock struct {
	path string
	file *os.File
}

func New(path string) *Lock {
	return &Lock{path: path}
}

func RootLock(root string) *Lock {
	return New(filepath.Join(root, "run", "operator.lock"))
}

func (l *Lock) Lock(ctx context.Context) error {
	if l.file != nil {
		return errors.New("lock is already held")
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			l.file = file
			return nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			file.Close()
			return fmt.Errorf("lock %s: %w", l.path, err)
		}
		select {
		case <-ctx.Done():
			file.Close()
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (l *Lock) Unlock() error {
	if l.file == nil {
		return nil
	}
	file := l.file
	l.file = nil
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (l *Lock) With(ctx context.Context, fn func() error) error {
	if err := l.Lock(ctx); err != nil {
		return err
	}
	defer l.Unlock()
	return fn()
}
