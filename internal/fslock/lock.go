package fslock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		err := tryLockFile(file)
		if err == nil {
			l.file = file
			return nil
		}
		if !isLockBusy(err) {
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
	if err := unlockFile(file); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (l *Lock) With(ctx context.Context, fn func() error) (err error) {
	if err := l.Lock(ctx); err != nil {
		return err
	}
	defer func() {
		if unlockErr := l.Unlock(); err == nil && unlockErr != nil {
			err = unlockErr
		}
	}()
	return fn()
}
