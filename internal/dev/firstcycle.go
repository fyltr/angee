package dev

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
)

// firstCycleScanner watches an io.Reader (typically the watcher's stdout
// pipe) for the locked first-cycle marker line. Closes `ready` exactly
// once when seen; closes `done` when the underlying reader EOFs without
// the marker (the orchestrator treats this as a failed start).
//
// Lines are also forwarded to `tee` so the user sees the watcher's
// progress in real time. The whole goroutine is cancellable via ctx.
type firstCycleScanner struct {
	src    io.Reader
	tee    io.Writer
	marker string
	ready  chan struct{}
	done   chan struct{}
}

func newFirstCycleScanner(
	src io.Reader, tee io.Writer, marker string,
) *firstCycleScanner {
	return &firstCycleScanner{
		src:    src,
		tee:    tee,
		marker: marker,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
}

func (s *firstCycleScanner) run(ctx context.Context) {
	defer close(s.done)
	signaledReady := false
	scan := bufio.NewScanner(s.src)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scan.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scan.Text()
		if s.tee != nil {
			_, _ = s.tee.Write([]byte(line + "\n"))
		}
		if !signaledReady && strings.Contains(line, s.marker) {
			signaledReady = true
			close(s.ready)
		}
	}
}

// Wait blocks until either the marker is seen, the reader closes, or
// the deadline expires. Returns true on marker; false otherwise.
//
// The deadline is generous: spawning Python + booting Django for the
// first build can take 10+ seconds on a cold venv.
func (s *firstCycleScanner) Wait(ctx context.Context, timeout time.Duration) bool {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-s.ready:
		return true
	case <-s.done:
		return false
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}
