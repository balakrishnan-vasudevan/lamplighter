# Lamplighter

Stream Kubernetes pod logs, events, and ingress side by side with synchronized time travel across all columns.

## What it does

When something breaks in production you end up with several terminals open: one tailing pod logs, one watching events, one in the ingress. You jump between them trying to reconstruct a timeline. Lamplighter puts all of that in one view.

The key feature is synchronized scrolling. When you scroll back in time, every column moves to the same timestamp. You see a consistent snapshot of your entire system at a single point in time.

## Install

Build from source:

```bash
git clone https://github.com/balakrishnan-vasudevan/lamplighter.git
cd lamplighter
go build -o lamplighter .
mv lamplighter /usr/local/bin/lamplighter
```

Or with `go install`:

```bash
go install github.com/balakrishnan-vasudevan/lamplighter@latest
```

## Usage

```bash
# Two pods side by side
lamplighter default/api-pod backend/worker

# Specific container
lamplighter default/api-pod:nginx

# All pods matching a label selector (adapts live as pods are created/deleted)
lamplighter default:selector:app=api

# Kubernetes events for a namespace
lamplighter default:events

# Ingress controller logs (auto-discovers nginx, traefik, kong, istio)
lamplighter default:ingress

# Custom ingress selector
lamplighter default:ingress:app=my-ingress

# Mix anything together
lamplighter default/api-pod default:selector:app=worker default:events

# Filter, tail, custom kubeconfig
lamplighter default/api-pod --filter "error|warn" --tail 100 --kubeconfig ~/.kube/staging
```

### Argument formats

| Format | What it streams |
|--------|----------------|
| `namespace/pod-name` | Single pod |
| `namespace/pod-name:container` | Specific container in a pod |
| `namespace:selector:label=value` | All pods matching the selector, live |
| `namespace:events` | Kubernetes events for the namespace |
| `namespace:ingress` | Ingress controller logs (auto-discovered) |
| `namespace:ingress:label=value` | Ingress controller with explicit selector |

## Keyboard controls

| Key | Action |
|-----|--------|
| `Tab` / `->` | Next column |
| `<-` | Previous column |
| `p` | Pause / unpause column |
| `f` | Focus column full-screen |
| `r` | Reconnect a dead or stuck column |
| `Up` | Scroll back in time (all columns move together) |
| `Down` | Scroll forward |
| `g` | Jump to oldest line in buffer |
| `G` | Return to live |
| `/` | Search (regex, matches as you type) |
| `Enter` | Lock search |
| `Esc` | Clear search |
| `e` | Export all columns to a timestamped `.log` file |
| `y` | Copy focused column to clipboard |
| `?` | Toggle keyboard help |
| `q` / `Ctrl+C` | Quit |

## Column status

```
● api-pod (ns:default) <
```

- `●` green: streaming
- `○` amber: reconnecting
- `x` red: dead (press `r` to reconnect)
- `<` marks the focused column
- `[PAUSED]` appears when the column display is frozen

## Log parsing

Plain text logs work as-is. JSON logs are detected automatically. The message, level, and timestamp are extracted regardless of which field names your logger uses (zap, logrus, structlog, and others all differ). Stack traces and multi-line output are folded under their parent line with a `(+N)` indicator.

## Search

Press `/` to open search. Matching lines stay bright, the rest dims. Matches against the display text, the raw log line, all parsed JSON fields, and stack trace lines. Searching for a trace ID finds the right line even if it was buried inside a JSON blob.

## Export and copy

Press `e` to export everything visible to a `lamplighter-<timestamp>.log` file in the current directory. Press `y` to copy the focused column to your system clipboard. The status bar confirms both.

## Label selector streaming

Selector columns watch the namespace for pod changes. When a pod matching the selector is created, a new log stream starts automatically. When a pod is deleted, its stream stops. All pods in a selector column write into the same column, prefixed with the pod name.

```bash
# Watch all pods in a rolling deploy
lamplighter default:selector:app=api
```

## Architecture

```
CLI (cobra)
  └─ App
       ├─ Manager: one goroutine per column
       │    ├─ Log: k8s log stream -> parse -> RingBuffer
       │    ├─ Selector: pod Watch -> one log goroutine per pod -> RingBuffer
       │    ├─ Events: k8s event Watch -> RingBuffer
       │    └─ Ingress: discover controller -> log stream -> RingBuffer
       └─ UI (bubbletea, 100ms tick)
            └─ reads RingBuffer snapshots -> renders columns
```

Each column owns a ring buffer (1000 lines). The UI reads a snapshot every 100ms. Streaming goroutines write independently and never block on the UI.

Selector columns run a pod Watch alongside the log streams. When a pod appears, a new log goroutine starts. When a pod is deleted, its goroutine is cancelled. All pod goroutines write into the same column buffer.

Exponential backoff on stream failure: 1s to 30s, resets on success. After 8 failures the column is marked dead.

## Not yet supported

- No `brew install` — build from source for now
- No demo mode — a real cluster is required to try it
- No Deployment or StatefulSet shorthand — you need to know the label selector (`default:selector:app=my-api` rather than `default:deploy:my-api`)
- No expand-to-columns — a label selector streams all matching pods into one shared column; there is no way to split them into one column per pod side by side
- No per-pod color coding within a selector column — pods are distinguished by a name prefix only
- No persistent config file — everything is flags on the command line every time

## License

MIT
