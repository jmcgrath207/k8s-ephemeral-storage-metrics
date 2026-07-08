package dev

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"k8s.io/client-go/rest"
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

func TestGetEnv(t *testing.T) {
	t.Run("set returns value", func(t *testing.T) {
		t.Setenv("TEST_KEY", "val")
		got := GetEnv("TEST_KEY", "fallback")
		if got != "val" {
			t.Errorf("expected 'val', got %q", got)
		}
	})
	t.Run("unset returns fallback", func(t *testing.T) {
		os.Unsetenv("TEST_UNSET")
		got := GetEnv("TEST_UNSET", "fall")
		if got != "fall" {
			t.Errorf("expected 'fall', got %q", got)
		}
	})
	t.Run("empty returns empty", func(t *testing.T) {
		t.Setenv("TEST_EMPTY", "")
		got := GetEnv("TEST_EMPTY", "fallback")
		if got != "" {
			t.Errorf("expected '', got %q", got)
		}
	})
}

func TestDeployAsDaemonSet(t *testing.T) {
	t.Run("unset defaults to DaemonSet", func(t *testing.T) {
		os.Unsetenv("DEPLOY_TYPE")
		if !DeployAsDaemonSet() {
			t.Error("expected true when unset (default DaemonSet)")
		}
	})
	t.Run("DaemonSet env returns true", func(t *testing.T) {
		t.Setenv("DEPLOY_TYPE", "DaemonSet")
		if !DeployAsDaemonSet() {
			t.Error("expected true for DaemonSet")
		}
	})
	t.Run("Deployment env returns false", func(t *testing.T) {
		t.Setenv("DEPLOY_TYPE", "Deployment")
		if DeployAsDaemonSet() {
			t.Error("expected false for Deployment")
		}
	})
	t.Run("empty string returns false", func(t *testing.T) {
		t.Setenv("DEPLOY_TYPE", "")
		if DeployAsDaemonSet() {
			t.Error("expected false for empty")
		}
	})
}

func TestCurrentNodeName(t *testing.T) {
	t.Run("unset returns empty", func(t *testing.T) {
		os.Unsetenv("CURRENT_NODE_NAME")
		if CurrentNodeName() != "" {
			t.Error("expected empty when unset")
		}
	})
	t.Run("returns env value", func(t *testing.T) {
		t.Setenv("CURRENT_NODE_NAME", "node-x")
		if CurrentNodeName() != "node-x" {
			t.Errorf("expected 'node-x', got %q", CurrentNodeName())
		}
	})
}

func TestLineInfoHookRun(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).Hook(LineInfoHook{})
	logger.Info().Msg("test")
	output := buf.String()
	if !strings.Contains(output, ".go") {
		t.Errorf("expected line info to contain .go, got: %s", output)
	}
}

func TestSetLogger_ValidLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")
	SetLogger()
}

func TestSetLogger_InvalidLevelPanics(t *testing.T) {
	t.Setenv("LOG_LEVEL", "garbage")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid log level")
		}
	}()
	SetLogger()
}

func TestSetScrapeFromKubelet(t *testing.T) {
	origClientRaw := ClientRaw
	origClientAno := ClientAno
	t.Cleanup(func() {
		ClientRaw = origClientRaw
		ClientAno = origClientAno
	})

	t.Setenv("SCRAPE_FROM_KUBELET_TLS_INSECURE_SKIP_VERIFY", "true")
	config := &rest.Config{Host: "https://localhost:6443"}
	setScrapeFromKubelet(config)
	if ClientRaw == nil {
		t.Error("ClientRaw should not be nil")
	}
	if ClientAno == nil {
		t.Error("ClientAno should not be nil")
	}
}

func TestSetK8sClient(t *testing.T) {
	origClientset := Clientset
	origClientRaw := ClientRaw
	origClientAno := ClientAno
	t.Cleanup(func() {
		Clientset = origClientset
		ClientRaw = origClientRaw
		ClientAno = origClientAno
	})

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

	t.Run("creates Clientset from kubeconfig", func(t *testing.T) {
		t.Setenv("KUBECONFIG", kubeconfig)
		t.Setenv("CLIENT_GO_QPS", "10")
		t.Setenv("CLIENT_GO_BURST", "20")
		SetK8sClient()
		if Clientset == nil {
			t.Error("Clientset should not be nil")
		}
	})

	t.Run("SCRAPE_FROM_KUBELET sets clients", func(t *testing.T) {
		t.Setenv("KUBECONFIG", kubeconfig)
		t.Setenv("SCRAPE_FROM_KUBELET", "true")
		SetK8sClient()
		if Clientset == nil {
			t.Error("Clientset should not be nil with SCRAPE_FROM_KUBELET")
		}
	})
}
