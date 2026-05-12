// Package dbg provides a simple file-based debug logger for aps.
// All functions are nil-safe no-ops when the logger has not been initialised.
package dbg

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

var (
	mu     sync.Mutex
	logger *log.Logger
	file   *os.File
)

// Open opens path for append-write and installs the package-level logger.
// Caller must call Close before the process exits.
func Open(path string) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	file = f
	logger = log.New(f, "", log.Ltime|log.Lmicroseconds)
	return nil
}

// Close flushes and closes the underlying file. Safe to call when not opened.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
		logger = nil
	}
}

// Writer returns the underlying io.Writer, or io.Discard if not opened.
func Writer() io.Writer {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		return file
	}
	return io.Discard
}

// Log writes a formatted line. No-op when logger is nil.
func Log(format string, args ...any) {
	mu.Lock()
	l := logger
	mu.Unlock()
	if l == nil {
		return
	}
	l.Output(2, fmt.Sprintf(format, args...)) //nolint:errcheck
}
