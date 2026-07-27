package cmd

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/app"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
)

var (
	pods       []string
	filter     string
	tail       int
	kubeconfig string
)

var rootCmd = &cobra.Command{
	Use:   "lamplighter [namespace/pod[:container] | namespace:selector:label=val | namespace:events | namespace:ingress[:selector]...]",
	Short: "Stream logs from multiple Kubernetes pods side by side",
	Example: `  lamplighter infra/proxy-pod backend/worker frontend/api
  lamplighter default:selector:app=api default:events
  lamplighter infra:ingress default/worker
  lamplighter --pod frontend/api-pod:app --pod backend/worker --filter "error|warn"
  lamplighter default/my-pod --tail 100`,
	Args: cobra.ArbitraryArgs,
	RunE: run,
}

func init() {
	rootCmd.Flags().StringArrayVar(&pods, "pod", nil, "namespace/pod-name[:container], repeatable (can also pass pods as positional args)")
	rootCmd.Flags().StringVar(&filter, "filter", "", "regex filter applied to all columns")
	rootCmd.Flags().IntVar(&tail, "tail", 0, "show last N lines on start (0 = follow from now)")
	rootCmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(_ *cobra.Command, args []string) error {
	allArgs := append(pods, args...)
	if len(allArgs) == 0 {
		return fmt.Errorf("at least one target required, e.g. lamplighter namespace/pod-name")
	}

	var filterRe *regexp.Regexp
	if filter != "" {
		var err error
		filterRe, err = regexp.Compile(filter)
		if err != nil {
			return fmt.Errorf("invalid --filter regex: %w", err)
		}
	}

	cols := make([]*column.Column, 0, len(allArgs))
	for i, arg := range allArgs {
		col, err := parseArg(arg, i, filterRe)
		if err != nil {
			return err
		}
		cols = append(cols, col)
	}

	a, err := app.New(cols, kubeconfig)
	if err != nil {
		return err
	}
	return a.Run()
}

// parseArg dispatches to the correct column type based on the argument format:
//
//	namespace/pod-name[:container]            → Log column
//	namespace:selector:label=val[,label=val]  → SelectorLog column
//	namespace:events                          → Events column
//	namespace:ingress[:label-selector]        → Ingress column
func parseArg(s string, idx int, filterRe *regexp.Regexp) (*column.Column, error) {
	id := fmt.Sprintf("col-%d", idx)
	buf := buffer.New(1000)

	// Check for the colon-separated type syntax first.
	// The distinguishing feature: the second segment is a known keyword
	// ("selector", "events", "ingress") or the first separator is "/".
	colonParts := strings.SplitN(s, ":", 3)
	if len(colonParts) >= 2 {
		ns := colonParts[0]
		keyword := strings.ToLower(colonParts[1])

		switch keyword {
		case "selector":
			if len(colonParts) < 3 || colonParts[2] == "" {
				return nil, fmt.Errorf("%q: selector syntax is namespace:selector:label=value", s)
			}
			return &column.Column{
				ID:        id,
				Type:      column.SelectorLog,
				Namespace: ns,
				Selector:  colonParts[2],
				Filter:    filterRe,
				Tail:      tail,
				Buffer:    buf,
			}, nil

		case "events":
			return &column.Column{
				ID:        id,
				Type:      column.Events,
				Namespace: ns,
				Filter:    filterRe,
				Buffer:    buf,
			}, nil

		case "ingress":
			sel := ""
			if len(colonParts) == 3 {
				sel = colonParts[2]
			}
			return &column.Column{
				ID:        id,
				Type:      column.Ingress,
				Namespace: ns,
				Selector:  sel,
				Filter:    filterRe,
				Buffer:    buf,
			}, nil
		}
	}

	// Fall back to namespace/pod-name[:container].
	return parsePodSpec(s, id, filterRe, buf)
}

// parsePodSpec parses "namespace/pod-name[:container]".
func parsePodSpec(s, id string, filterRe *regexp.Regexp, buf *buffer.RingBuffer) (*column.Column, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("%q: expected namespace/pod-name[:container] or namespace:selector:...", s)
	}
	ns := parts[0]
	rest := parts[1]

	podName := rest
	var container string
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		podName = rest[:i]
		container = rest[i+1:]
	}

	if ns == "" || podName == "" {
		return nil, fmt.Errorf("%q: namespace and pod name must not be empty", s)
	}

	return &column.Column{
		ID:        id,
		Type:      column.Log,
		Namespace: ns,
		PodName:   podName,
		Container: container,
		Filter:    filterRe,
		Tail:      tail,
		Buffer:    buf,
	}, nil
}
