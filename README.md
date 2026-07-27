# Lamplighter

> *A light in dark places*

Stream logs from multiple Kubernetes pods and namespaces simultaneously, displayed in side-by-side columns in your terminal.

## What it does

- Multiple pods side-by-side in one terminal window
- Per-column color coding and status indicators
- Log level highlighting (ERROR in red, WARN in amber)
- Regex filter applied across all columns
- Automatic reconnect on stream failure with exponential backoff
- Pause, focus, and reconnect individual columns via keyboard

## Install

```bash
go install github.com/balakrishnan-vasudevan/lamplighter@latest
```

Or build from source:

```bash
make build
./lamplighter --help
```

## Usage

```bash
# Two pods side by side
lamplighter --pod frontend/api-pod-abc --pod backend/worker-pod-xyz

# Specific container within a pod
lamplighter --pod frontend/api-pod-abc:nginx --pod backend/worker-pod-xyz:app

# Filter logs matching a regex across all columns
lamplighter --pod frontend/api-pod-abc --pod backend/worker-pod-xyz --filter "error|warn"

# Tail last 100 lines on start, then follow
lamplighter --pod frontend/api-pod-abc --pod backend/worker-pod-xyz --tail 100

# Custom kubeconfig
lamplighter --pod default/my-pod --kubeconfig ~/.kube/staging-config
```

## Keyboard controls

| Key | Action |
|-----|--------|
| `Tab` / `→` | Move cursor to next column |
| `←` | Move cursor to previous column |
| `p` | Pause / unpause cursor column |
| `f` | Focus cursor column full-screen (press again to exit) |
| `r` | Reconnect cursor column (useful when status shows Dead) |
| `?` | Toggle keyboard help |
| `q` / `Ctrl+C` | Quit |

## Column header

Each column header shows:

```
● api-pod-abc (ns:frontend) ◀
```

- `●` green = streaming, `○` amber = reconnecting, `✕` red = dead
- `◀` marks the currently selected column (keyboard target)
- `[PAUSED]` appears when the column's scroll is frozen

## Architecture

```
CLI (cobra)
  └─ App (orchestrator, root context)
       ├─ StreamManager — one goroutine per column
       │    └─ k8s log stream → filter → RingBuffer (1000 lines, circular)
       └─ Renderer (bubbletea, 100ms tick)
            └─ reads RingBuffer snapshots → draws columns
```

Key design properties:

- **RingBuffer** — goroutine-safe circular buffer per column; renderer always reads the last N lines where N = terminal height. Old lines are silently evicted.
- **Reconnect with backoff** — exponential backoff from 1s to 30s; after 8 failures the column is marked Dead. Press `r` to restart the stream.
- **Zero-copy rendering** — renderer reads a snapshot of the buffer every 100ms; streaming goroutines never block on the UI.
- **Graceful shutdown** — cancelling the root context stops all streaming goroutines; bubbletea cleans up the terminal.

## Tech stack

- [`client-go`](https://github.com/kubernetes/client-go) — Kubernetes API streaming
- [`bubbletea`](https://github.com/charmbracelet/bubbletea) — TUI framework
- [`lipgloss`](https://github.com/charmbracelet/lipgloss) — terminal styling
- [`cobra`](https://github.com/spf13/cobra) — CLI
