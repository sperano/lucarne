package kube

import (
	"context"
	"errors"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// userAgent names Lucarne in the API server's logs.
const userAgent = "lucarne"

// LogStreamer opens a container's log. PodLogs is the client-go one; a test
// gives its own, since client-go's fake serves a fixed text.
type LogStreamer interface {
	Stream(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error)
}

// PodLogs streams logs through the pods/log subresource.
type PodLogs struct{ Kube kubernetes.Interface }

// Stream opens the log of pod as opts says.
func (p PodLogs) Stream(ctx context.Context, namespace, pod string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	return p.Kube.CoreV1().Pods(namespace).GetLogs(pod, opts).Stream(ctx)
}

// RESTConfig is the in-cluster configuration, or outside a cluster the
// kubeconfig's current context ($KUBECONFIG, else ~/.kube/config).
func RESTConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if errors.Is(err, rest.ErrNotInCluster) {
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("kube: client configuration: %w", err)
	}
	return cfg, nil
}

// NewClients builds the source's clients from cfg.
func NewClients(cfg *rest.Config) (Clients, error) {
	cfg = rest.CopyConfig(cfg)
	cfg.UserAgent = userAgent
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return Clients{}, fmt.Errorf("kube: dynamic client: %w", err)
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return Clients{}, fmt.Errorf("kube: client: %w", err)
	}
	return Clients{Dynamic: dyn, Kube: kube, Logs: PodLogs{Kube: kube}}, nil
}
