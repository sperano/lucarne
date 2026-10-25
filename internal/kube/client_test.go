package kube

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestPodLogsStream proves the production streamer only gets the pods/log
// subresource, with the options it was given.
func TestPodLogsStream(t *testing.T) {
	kube := kubefake.NewClientset()
	opts := &corev1.PodLogOptions{Container: AgentContainer, Timestamps: true}
	body, err := PodLogs{Kube: kube}.Stream(context.Background(), testNamespace, podName, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if data, err := io.ReadAll(body); err != nil || len(data) == 0 {
		t.Fatalf("read %q, %v", data, err)
	}
	actions := kube.Actions()
	if len(actions) != 1 {
		t.Fatalf("actions = %+v", actions)
	}
	a := actions[0].(k8stesting.GenericAction)
	if a.GetVerb() != "get" || a.GetResource().Resource != "pods" || a.GetSubresource() != "log" || a.GetNamespace() != testNamespace || a.GetValue() != opts {
		t.Errorf("action = %+v", a)
	}
}

const testKubeconfig = `apiVersion: v1
kind: Config
clusters:
- name: example
  cluster:
    server: https://cluster.example.com
contexts:
- name: example
  context:
    cluster: example
    user: viewer
current-context: example
users:
- name: viewer
  user:
    token: fabricated
`

func TestRESTConfigOutsideCluster(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte(testKubeconfig), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", path)
	cfg, err := RESTConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "https://cluster.example.com" {
		t.Errorf("host = %q", cfg.Host)
	}
	clients, err := NewClients(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserAgent != "" {
		t.Error("NewClients changed the configuration it was given")
	}
	if clients.Dynamic == nil || clients.Kube == nil || clients.Logs == nil {
		t.Errorf("clients = %+v", clients)
	}
}
