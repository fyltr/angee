package dev

import (
	"bufio"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

// LineSink is the line-based output channel for `angee dev --ui=lines`.
// Each child gets its own *prefixWriter (tee'd onto stdout/stderr); the
// sink serialises writes so lines never interleave mid-line and applies a
// stable colour per child name when the underlying writer is a TTY.
//
// This is the default output mode. Pane-mode (`--ui=panes`, Phase 6) wraps
// the same Sink interface around a bubbletea program.
type LineSink struct {
	mu       sync.Mutex
	out      io.Writer
	useColor bool
}

// Sink is the minimal contract the orchestrator needs from an output
// strategy. Both LineSink and the future PaneSink implement this so the
// orchestrator code is identical across UI modes.
type Sink interface {
	// Writer returns a child-specific writer. Each line written gets a
	// `[name]` prefix in lines-mode or routed to the child's pane in
	// panes-mode. Safe for concurrent use.
	Writer(name string) io.Writer

	// SystemLine prints an orchestrator-level message (start, stop,
	// child exited rc=N, …) prefixed `[angee]`.
	SystemLine(format string, args ...any)
}

// NewLineSink returns a LineSink writing to out. Colour is enabled when
// out is os.Stdout and stdout is a TTY (or when forceColor is true).
func NewLineSink(out io.Writer, forceColor bool) *LineSink {
	useColor := forceColor || isStdoutTTY(out)
	return &LineSink{out: out, useColor: useColor}
}

// Writer returns a per-child writer. The returned io.Writer can be
// assigned to cmd.Stdout / cmd.Stderr directly; the prefixWriter buffers
// its input by line so a partial write doesn't produce a half-prefixed
// line.
func (s *LineSink) Writer(name string) io.Writer {
	pw, w := io.Pipe()
	pre := s.colorPrefix(name)
	go func() {
		defer pw.Close()
		scan := bufio.NewScanner(pw)
		// Default Scanner buffer is 64 KiB; we extend to 1 MiB to be
		// safe for very long log lines (think Django tracebacks).
		scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scan.Scan() {
			s.mu.Lock()
			fmt.Fprintf(s.out, "%s %s\n", pre, scan.Text())
			s.mu.Unlock()
		}
	}()
	return w
}

// SystemLine prints an orchestrator-level message prefixed `[angee]`.
func (s *LineSink) SystemLine(format string, args ...any) {
	pre := s.colorPrefix("angee")
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.out, "%s %s\n", pre, fmt.Sprintf(format, args...))
}

// palette is the 8-colour ANSI ring used for prefix colouring. Indexed
// by `crc32(name) % len(palette)` so each name keeps a stable colour
// across runs (good for skimming output in CI / replays).
var palette = []string{
	"\033[36m", // cyan
	"\033[33m", // yellow
	"\033[35m", // magenta
	"\033[32m", // green
	"\033[34m", // blue
	"\033[31m", // red
	"\033[37m", // white
	"\033[90m", // dim
}

func (s *LineSink) colorPrefix(name string) string {
	tag := "[" + name + "]"
	if !s.useColor {
		return tag
	}
	i := int(crc32.ChecksumIEEE([]byte(name))) % len(palette)
	return palette[i] + tag + "\033[0m"
}

// isStdoutTTY checks if w is os.Stdout AND that stdout is a terminal.
// We don't enable colour for arbitrary writers (e.g. CI log capture).
func isStdoutTTY(w io.Writer) bool {
	if w != os.Stdout {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
