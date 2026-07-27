package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/client-go/kubernetes"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
	k8sclient "github.com/balakrishnan-vasudevan/lamplighter/internal/k8s"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/stream"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/ui"
)

type App struct {
	columns []*column.Column
	manager *stream.Manager
	client  kubernetes.Interface
	cancel  context.CancelFunc
}

func New(cols []*column.Column, kubeconfig string) (*App, error) {
	client, err := k8sclient.NewClient(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("connecting to Kubernetes: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	mgr := stream.NewManager(ctx, client)

	return &App{
		columns: cols,
		manager: mgr,
		client:  client,
		cancel:  cancel,
	}, nil
}

func (a *App) Run() error {
	defer a.cancel()

	a.manager.Start(a.columns)

	model := ui.New(a.columns, a.manager)
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Cancel streams on SIGTERM so goroutines exit cleanly.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() {
		<-sigs
		a.cancel()
		p.Quit()
	}()

	_, err := p.Run()
	return err
}
