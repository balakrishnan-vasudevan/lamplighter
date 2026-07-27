package column

import (
	"fmt"
	"regexp"
	"sync"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
)

type Type int

const (
	Log         Type = iota // single pod logs
	SelectorLog             // all pods matching a label selector
	Events                  // namespace events via k8s watch
	Ingress                 // ingress controller access logs (auto-discovered)
)

type Status int

const (
	Streaming    Status = iota
	Reconnecting Status = iota
	Dead         Status = iota
)

type Column struct {
	ID        string
	Type      Type
	Namespace string
	PodName   string // Log columns only
	Container string // optional
	Selector  string // SelectorLog and Ingress columns
	Filter    *regexp.Regexp
	Tail      int
	Buffer    *buffer.RingBuffer

	mu     sync.RWMutex
	status Status
}

func (c *Column) SetStatus(s Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
}

func (c *Column) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

func (c *Column) Header() string {
	switch c.Type {
	case Log:
		name := c.PodName
		if c.Container != "" {
			name += ":" + c.Container
		}
		return fmt.Sprintf("%s (ns:%s)", name, c.Namespace)
	case SelectorLog:
		return fmt.Sprintf("[sel] %s (ns:%s)", c.Selector, c.Namespace)
	case Events:
		return fmt.Sprintf("[evt] (ns:%s)", c.Namespace)
	case Ingress:
		return fmt.Sprintf("[ing] (ns:%s)", c.Namespace)
	}
	return c.PodName
}
