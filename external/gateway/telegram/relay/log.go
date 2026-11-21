//go:build gateway || gateway.telegram

package relay

import (
	"log"
	"os"
	"path/filepath"
	"sync"
)

// relayLogLimit bounds relay.log: a file grown past it is truncated when the
// next worker starts. Failures are rate-limited by their writers, so the file
// stays well under the bound in normal use.
const relayLogLimit = 1 << 20

// workerLog appends timestamped lines to relay.log next to the discovery
// files. The worker is detached, so this file is its only diagnostics surface;
// a nil workerLog swallows writes instead.
type workerLog struct {
	mu   sync.Mutex
	file *os.File
	log  *log.Logger
}

func openWorkerLog(dir string) *workerLog {
	path := filepath.Join(dir, "relay.log")
	flags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if info, err := os.Stat(path); err == nil && info.Size() > relayLogLimit {
		flags = os.O_TRUNC | os.O_CREATE | os.O_WRONLY
	}
	f, err := os.OpenFile(path, flags, 0o600) // #nosec G304 -- relay-owned user cache file
	if err != nil {
		return nil
	}
	return &workerLog{file: f, log: log.New(f, "", log.LstdFlags)}
}

func (l *workerLog) write(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log.Printf(format, args...)
}

func (l *workerLog) close() {
	if l == nil {
		return
	}
	_ = l.file.Close()
}
