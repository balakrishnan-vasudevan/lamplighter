package stream

import (
	"bufio"
	"context"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/logparser"
)

const (
	maxBackoff = 30 * time.Second
	maxRetries = 8
)

// streamPodLogs streams logs from a specific pod into col.Buffer.
// podName is passed explicitly so selector columns can fan out to multiple pods
// while all writing to the same buffer.
func streamPodLogs(ctx context.Context, client kubernetes.Interface, col *column.Column, podName string) {
	backoff := time.Second
	retries := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		opts := &corev1.PodLogOptions{
			Follow:     true,
			Timestamps: true,
		}
		if col.Container != "" {
			opts.Container = col.Container
		}
		if col.Tail > 0 {
			tail := int64(col.Tail)
			opts.TailLines = &tail
		}

		req := client.CoreV1().Pods(col.Namespace).GetLogs(podName, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			col.SetStatus(column.Reconnecting)
			retries++
			if retries >= maxRetries {
				col.SetStatus(column.Dead)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
			}
			continue
		}

		retries = 0
		backoff = time.Second

		var pending *buffer.Line

		scanner := bufio.NewScanner(stream)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				stream.Close()
				if pending != nil && matchesFilter(col.Filter, pending) {
					col.Buffer.Write(*pending)
				}
				return
			default:
			}

			raw := scanner.Text()

			// Fold continuation lines (stack traces, wrapped output) into the previous line.
			if pending != nil && logparser.IsContinuation(raw) {
				pending.Extra = append(pending.Extra, raw)
				continue
			}

			// Flush previous line.
			if pending != nil && matchesFilter(col.Filter, pending) {
				col.Buffer.Write(*pending)
			}

			line := logparser.ParseLine(raw)

			// For selector columns, prefix the pod name so the source is visible.
			if col.Type == column.SelectorLog {
				line.Text = shortName(podName) + " " + line.Text
			}

			pending = &line
		}

		stream.Close()

		if pending != nil && matchesFilter(col.Filter, pending) {
			col.Buffer.Write(*pending)
		}

		if ctx.Err() != nil {
			return
		}

		col.SetStatus(column.Reconnecting)
		retries++
		if retries >= maxRetries {
			col.SetStatus(column.Dead)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

func matchesFilter(re *regexp.Regexp, line *buffer.Line) bool {
	if re == nil {
		return true
	}
	if re.MatchString(line.Text) {
		return true
	}
	if line.RawText != "" && re.MatchString(line.RawText) {
		return true
	}
	for _, v := range line.Fields {
		if re.MatchString(v) {
			return true
		}
	}
	return false
}

// shortName returns the last segment of a pod name to keep selector column prefixes compact.
// "api-pod-f8d2c-xkz9p" → "xkz9p"
func shortName(podName string) string {
	parts := splitLast(podName, "-")
	if len(parts[len(parts)-1]) >= 5 {
		return "[" + parts[len(parts)-1] + "]"
	}
	// If last segment is short, use last two segments.
	if len(parts) >= 2 {
		return "[" + parts[len(parts)-2] + "-" + parts[len(parts)-1] + "]"
	}
	return "[" + podName + "]"
}

func splitLast(s, sep string) []string {
	parts := make([]string, 0)
	for {
		idx := lastIndex(s, sep)
		if idx < 0 {
			parts = append([]string{s}, parts...)
			break
		}
		parts = append([]string{s[idx+len(sep):]}, parts...)
		s = s[:idx]
	}
	return parts
}

func lastIndex(s, sep string) int {
	for i := len(s) - len(sep); i >= 0; i-- {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
}
