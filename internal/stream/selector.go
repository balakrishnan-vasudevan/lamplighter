package stream

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
)

// streamSelector watches pods matching col.Selector and maintains one
// streamPodLogs goroutine per running pod, all writing to the same col.Buffer.
func streamSelector(ctx context.Context, client kubernetes.Interface, col *column.Column) {
	podCancels := make(map[string]context.CancelFunc)

	defer func() {
		for _, cancel := range podCancels {
			cancel()
		}
	}()

	startPod := func(podName string) {
		if _, ok := podCancels[podName]; ok {
			return
		}
		podCtx, podCancel := context.WithCancel(ctx)
		podCancels[podName] = podCancel
		go streamPodLogs(podCtx, client, col, podName)
	}

	stopPod := func(podName string) {
		if cancel, ok := podCancels[podName]; ok {
			cancel()
			delete(podCancels, podName)
		}
	}

	// List existing running pods immediately.
	list, err := client.CoreV1().Pods(col.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: col.Selector,
	})
	if err == nil {
		for _, pod := range list.Items {
			if pod.DeletionTimestamp == nil {
				startPod(pod.Name)
			}
		}
	}

	// Watch for future pod changes.
	backoff := time.Second
	for {
		resourceVersion := ""
		if list != nil {
			resourceVersion = list.ResourceVersion
		}

		watcher, err := client.CoreV1().Pods(col.Namespace).Watch(ctx, metav1.ListOptions{
			LabelSelector:   col.Selector,
			ResourceVersion: resourceVersion,
			Watch:           true,
		})
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			}
		}

		backoff = time.Second
		list = nil

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

				switch event.Type {
				case watch.Added, watch.Modified:
					if pod, ok := getPodName(event); ok {
						startPod(pod)
					}
				case watch.Deleted:
					if pod, ok := getPodName(event); ok {
						stopPod(pod)
					}
				}
			}
		}

	reconnect:
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

func getPodName(event watch.Event) (string, bool) {
	type namedObject interface {
		GetName() string
	}
	if o, ok := event.Object.(namedObject); ok {
		name := o.GetName()
		return name, name != ""
	}
	return "", false
}

// fmtDuration returns a short human-readable duration for display in events.
func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
