package buffer

import (
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelInfo  LogLevel = iota
	LevelWarn
	LevelError
)

func ParseLevel(text string) LogLevel {
	u := strings.ToUpper(text)
	if strings.Contains(u, "ERROR") || strings.Contains(u, "FATAL") || strings.Contains(u, "CRIT") {
		return LevelError
	}
	if strings.Contains(u, "WARN") {
		return LevelWarn
	}
	return LevelInfo
}

// ParseLevelString converts an explicit level field value (from JSON logs) to LogLevel.
func ParseLevelString(s string) LogLevel {
	switch strings.ToUpper(s) {
	case "ERROR", "ERR", "FATAL", "CRITICAL", "CRIT", "EMERG", "ALERT", "50", "60":
		return LevelError
	case "WARN", "WARNING", "40":
		return LevelWarn
	default:
		return LevelInfo
	}
}

type Line struct {
	Timestamp time.Time
	Text      string            // formatted display text
	RawText   string            // original log line (used for search matching)
	Level     LogLevel
	Fields    map[string]string // parsed JSON fields; nil for plain-text lines
	Extra     []string          // continuation lines (stack traces, multi-line output)
}

type RingBuffer struct {
	mu    sync.RWMutex
	lines []Line
	head  int
	count int
	size  int
}

func New(size int) *RingBuffer {
	return &RingBuffer{lines: make([]Line, size), size: size}
}

func (r *RingBuffer) Write(line Line) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines[r.head] = line
	r.head = (r.head + 1) % r.size
	if r.count < r.size {
		r.count++
	}
}

// Read returns the last n lines in chronological order (oldest first).
func (r *RingBuffer) Read(n int) []Line {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if n > r.count {
		n = r.count
	}
	if n == 0 {
		return nil
	}
	out := make([]Line, n)
	start := ((r.head - n) % r.size + r.size) % r.size
	for i := 0; i < n; i++ {
		out[i] = r.lines[(start+i)%r.size]
	}
	return out
}

func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.count
}

// ReadBefore returns up to n lines whose Timestamp <= t, in chronological order.
// Assumes lines are written in approximately chronological order (k8s guarantees this).
func (r *RingBuffer) ReadBefore(t time.Time, n int) []Line {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.count == 0 || n == 0 {
		return nil
	}

	base := ((r.head - r.count) % r.size + r.size) % r.size

	// Scan backwards from newest to find the cutoff point.
	cutoff := 0
	for i := r.count - 1; i >= 0; i-- {
		if !r.lines[(base+i)%r.size].Timestamp.After(t) {
			cutoff = i + 1
			break
		}
	}

	if cutoff == 0 {
		return nil
	}
	if n > cutoff {
		n = cutoff
	}

	out := make([]Line, n)
	start := cutoff - n
	for i := 0; i < n; i++ {
		out[i] = r.lines[(base+start+i)%r.size]
	}
	return out
}
