package stream

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
)

// streamEvents watches k8s Events in col.Namespace and writes formatted
// event lines into col.Buffer.
func streamEvents(ctx context.Context, client kubernetes.Interface, col *column.Column) {
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		col.SetStatus(column.Streaming)

		watcher, err := client.CoreV1().Events(col.Namespace).Watch(ctx, metav1.ListOptions{
			Watch: true,
		})
		if err != nil {
			col.SetStatus(column.Reconnecting)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
			}
			continue
		}

		backoff = time.Second

		for {
			select {
			case <-ctx.Done():
				watcher.Stop()
				return

			case event, ok := <-watcher.ResultChan():
				if !ok {
					watcher.Stop()
					goto reconnect
				}

				if event.Type == watch.Added || event.Type == watch.Modified {
					if ev, ok := event.Object.(*corev1.Event); ok {
						line := formatEvent(ev)
						col.Buffer.Write(line)
					}
				}
			}
		}

	reconnect:
		col.SetStatus(column.Reconnecting)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

func formatEvent(ev *corev1.Event) buffer.Line {
	ts := ev.LastTimestamp.Time
	if ts.IsZero() {
		ts = ev.EventTime.Time
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	age := fmtDuration(time.Since(ts))
	count := ""
	if ev.Count > 1 {
		count = fmt.Sprintf(" x%d", ev.Count)
	}

	kind := ev.InvolvedObject.Kind
	name := ev.InvolvedObject.Name
	// Shorten pod names for readability.
	if kind == "Pod" {
		name = shortName(name)
	}

	reason := strings.ToUpper(ev.Reason)
	text := fmt.Sprintf("[%s] %s/%s  %s%s  (%s)", reason, kind, name, ev.Message, count, age)

	lvl := buffer.LevelInfo
	if ev.Type == "Warning" {
		lvl = buffer.LevelWarn
	}

	return buffer.Line{
		Timestamp: ts,
		Text:      text,
		RawText:   text,
		Level:     lvl,
	}
}
