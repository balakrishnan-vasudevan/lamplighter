package stream

import (
	"context"
	"sync"

	"k8s.io/client-go/kubernetes"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
)

type Manager struct {
	client  kubernetes.Interface
	rootCtx context.Context

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
	wg      sync.WaitGroup
}

func NewManager(ctx context.Context, client kubernetes.Interface) *Manager {
	return &Manager{
		client:  client,
		rootCtx: ctx,
		cancels: make(map[string]context.CancelFunc),
	}
}

func (m *Manager) Start(cols []*column.Column) {
	for _, col := range cols {
		m.startColumn(col)
	}
}

// Reconnect cancels any running goroutine for the column and starts a fresh one.
func (m *Manager) Reconnect(col *column.Column) {
	col.SetStatus(column.Reconnecting)
	m.startColumn(col)
}

func (m *Manager) startColumn(col *column.Column) {
	m.mu.Lock()
	if cancel, ok := m.cancels[col.ID]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(m.rootCtx)
	m.cancels[col.ID] = cancel
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		switch col.Type {
		case column.SelectorLog:
			streamSelector(ctx, m.client, col)
		case column.Events:
			streamEvents(ctx, m.client, col)
		case column.Ingress:
			streamIngress(ctx, m.client, col)
		default:
			streamPodLogs(ctx, m.client, col, col.PodName)
		}
	}()
}

func (m *Manager) Wait() {
	m.wg.Wait()
}
