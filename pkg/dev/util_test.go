package dev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetK8sConfigFromEnv_Unset(t *testing.T) {
	os.Unsetenv("KUBECONFIG")
	cfg, err := getK8sConfigFromEnv()
	if err != nil {
		t.Errorf("expected nil error when KUBECONFIG unset, got %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config when KUBECONFIG unset, got %v", cfg)
	}
}

func TestGetK8sConfigFromEnv_ValidPath(t *testing.T) {
	dir := t.TempDir()
	kubeconfig := filepath.Join(dir, "config")
	content := `apiVersion: v1
kind: Config
current-context: test
clusters:
- cluster:
    server: https://localhost:6443
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
  name: test
users:
- name: test-user
  user:
    token: fake-token
`
	if err := os.WriteFile(kubeconfig, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)
	cfg, err := getK8sConfigFromEnv()
	if err != nil {
		t.Fatalf("expected nil error for valid kubeconfig, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config for valid kubeconfig")
	}
}

func TestGetK8sConfigFromEnv_InvalidPath(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path/kubeconfig")
	cfg, err := getK8sConfigFromEnv()
	if err == nil {
		t.Error("expected error for invalid kubeconfig path")
	}
	if cfg != nil {
		t.Errorf("expected nil config on error, got %v", cfg)
	}
}

func TestGetK8sConfig_BadKubeconfigPath(t *testing.T) {
	t.Setenv("KUBECONFIG", "/nonexistent/path/kubeconfig")
	_, err := getK8sConfig()
	if err == nil {
		t.Fatal("expected error for bad KUBECONFIG path")
	}
	if !strings.Contains(err.Error(), "KUBECONFIG set but invalid") {
		t.Errorf("error should contain 'KUBECONFIG set but invalid', got: %v", err)
	}
}

func TestGetK8sConfig_FallbackToInCluster(t *testing.T) {
	os.Unsetenv("KUBECONFIG")
	cfg, err := getK8sConfig()
	if err != nil {
		// Expected when not running in a cluster
		t.Logf("getK8sConfig() fallback: %v (expected outside cluster)", err)
		return
	}
	t.Log("got in-cluster config")
	_ = cfg
}
