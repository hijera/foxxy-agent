package serve

import (
	"sync"
	"sync/atomic"

	"github.com/hijera/foxxycode-agent/internal/config"
)

// liveConfig holds the configuration of a process that has no session manager
// - a relay alone - and answers the questions the manager answers for the
// others: what is live, replace it, who is watching. Without it such a process
// ran the file it started with for good: a relay's settings saved from its own
// page, or its file edited by hand, reached nothing until a restart (issue
// #401).
type liveConfig struct {
	cur atomic.Pointer[config.Config]

	// replaceMu holds one replacement from its store to its last observer, so
	// the supervisor hears replacements in the order they were stored.
	replaceMu sync.Mutex

	mu        sync.Mutex
	seq       uint64
	observers map[uint64]func(*config.Config)
}

func (l *liveConfig) load() *config.Config { return l.cur.Load() }

// replace installs next and tells every observer. Like the manager's, an
// observer runs on the replacing goroutine, hears the replacements one at a
// time in the order they were stored, and must neither block nor replace the
// configuration itself.
func (l *liveConfig) replace(next *config.Config) {
	if next == nil {
		return
	}
	l.replaceMu.Lock()
	defer l.replaceMu.Unlock()
	l.cur.Store(next)
	l.mu.Lock()
	fns := make([]func(*config.Config), 0, len(l.observers))
	for _, fn := range l.observers {
		fns = append(fns, fn)
	}
	l.mu.Unlock()
	for _, fn := range fns {
		fn(next)
	}
}

func (l *liveConfig) observe(fn func(*config.Config)) (remove func()) {
	if fn == nil {
		return func() {}
	}
	l.mu.Lock()
	if l.observers == nil {
		l.observers = make(map[uint64]func(*config.Config))
	}
	l.seq++
	id := l.seq
	l.observers[id] = fn
	l.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			delete(l.observers, id)
			l.mu.Unlock()
		})
	}
}

// ReplaceConfig installs a reloaded configuration: through the session manager
// when the process has one, which every surface already observes, else in the
// runtime's own holder.
func (r *Runtime) ReplaceConfig(next *config.Config) {
	if next == nil {
		return
	}
	if r.Mgr != nil {
		r.Mgr.ReplaceConfig(next)
		return
	}
	r.live.replace(next)
}

// AddConfigObserver registers fn for every replacement of the live
// configuration, whoever holds it, and returns the function that removes it.
// fn must not block.
func (r *Runtime) AddConfigObserver(fn func(*config.Config)) (remove func()) {
	if r.Mgr != nil {
		return r.Mgr.AddConfigObserver(fn)
	}
	return r.live.observe(fn)
}
