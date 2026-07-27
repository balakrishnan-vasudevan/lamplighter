package stream

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/balakrishnan-vasudevan/lamplighter/internal/buffer"
	"github.com/balakrishnan-vasudevan/lamplighter/internal/column"
)

// Well-known ingress controller label selectors, tried in order.
var ingressSelectors = []struct {
	namespace string // empty = col.Namespace
	selector  string
}{
	{selector: "app.kubernetes.io/name=ingress-nginx"},
	{selector: "app=ingress-nginx"},
	{selector: "app.kubernetes.io/name=traefik"},
	{selector: "app=traefik"},
	{selector: "app=kong"},
	{selector: "app=istio-ingressgateway"},
	{selector: "app=envoy"},
}

// Well-known ingress namespaces to search in if the column namespace yields nothing.
var ingressNamespaces = []string{"ingress-nginx", "kube-system", "traefik", "istio-system", "kong"}

// streamIngress auto-discovers the ingress controller pod(s) and streams their logs.
// If col.Selector is set explicitly, it uses that; otherwise it tries the well-known selectors.
func streamIngress(ctx context.Context, client kubernetes.Interface, col *column.Column) {
	podName, ns, err := discoverIngress(ctx, client, col)
	if err != nil {
		col.Buffer.Write(buffer.Line{
			Timestamp: time.Now(),
			Text:      fmt.Sprintf("[ing] discovery failed: %s", err),
			Level:     buffer.LevelWarn,
		})
		col.SetStatus(column.Dead)
		return
	}

	// Reuse selector streaming if we found a label selector, otherwise stream by pod name.
	syntheticCol := *col
	syntheticCol.Namespace = ns
	syntheticCol.Type = column.SelectorLog
	syntheticCol.Selector = "app=" + podName // may be overridden below

	if col.Selector != "" {
		syntheticCol.Selector = col.Selector
		streamSelector(ctx, client, &syntheticCol)
	} else {
		// Stream the specific pod we found.
		syntheticCol.Type = column.Log
		streamPodLogs(ctx, client, &syntheticCol, podName)
	}
}

// discoverIngress returns the first pod name (and its namespace) that matches
// any of the well-known ingress selectors.
func discoverIngress(ctx context.Context, client kubernetes.Interface, col *column.Column) (podName, ns string, err error) {
	namespaces := []string{col.Namespace}
	for _, n := range ingressNamespaces {
		if n != col.Namespace {
			namespaces = append(namespaces, n)
		}
	}

	for _, candidate := range ingressSelectors {
		searchNS := namespaces
		if candidate.namespace != "" {
			searchNS = []string{candidate.namespace}
		}

		for _, n := range searchNS {
			list, lerr := client.CoreV1().Pods(n).List(ctx, metav1.ListOptions{
				LabelSelector: candidate.selector,
			})
			if lerr != nil || len(list.Items) == 0 {
				continue
			}
			for _, pod := range list.Items {
				if pod.DeletionTimestamp == nil {
					return pod.Name, n, nil
				}
			}
		}
	}

	return "", "", fmt.Errorf("no ingress controller pods found in %v", namespaces)
}
