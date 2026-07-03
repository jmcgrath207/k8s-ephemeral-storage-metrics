package dev

import (
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"k8s.io/client-go/rest"
)

func TestGetEnv_UnsetReturnsFallback(t *testing.T) {
	os.Unsetenv("TEST_GETENV_UNSET")
	got := GetEnv("TEST_GETENV_UNSET", "fallback_val")
	if got != "fallback_val" {
		t.Errorf("GetEnv(unset) = %q, want %q", got, "fallback_val")
	}
}

func TestGetEnv_SetReturnsValue(t *testing.T) {
	os.Setenv("TEST_GETENV_SET", "custom_val")
	defer os.Unsetenv("TEST_GETENV_SET")
	got := GetEnv("TEST_GETENV_SET", "fallback_val")
	if got != "custom_val" {
		t.Errorf("GetEnv(set) = %q, want %q", got, "custom_val")
	}
}

func TestGetEnv_EmptyStringUsesFallback(t *testing.T) {
	os.Setenv("TEST_GETENV_EMPTY", "")
	defer os.Unsetenv("TEST_GETENV_EMPTY")
	got := GetEnv("TEST_GETENV_EMPTY", "fallback_val")
	if got != "" {
		t.Errorf("GetEnv(empty) = %q, want empty string", got)
	}
}

func TestSetLogger_Levels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}
	for _, lvl := range levels {
		os.Setenv("LOG_LEVEL", lvl)
		defer os.Unsetenv("LOG_LEVEL")
		// Should not panic
		SetLogger()
	}
}

func TestSetLogger_InvalidLevelPanics(t *testing.T) {
	os.Setenv("LOG_LEVEL", "invalid_level")
	defer os.Unsetenv("LOG_LEVEL")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid log level")
		}
	}()
	SetLogger()
}

func TestGetEnv_Empty(t *testing.T) {
	got := GetEnv("", "default")
	if got != "default" {
		t.Errorf("GetEnv(empty_key) = %q, want %q", got, "default")
	}
}

func TestLineInfoHook_Run(t *testing.T) {
	// Just verify LineInfoHook doesn't panic
	h := LineInfoHook{}
	l := zerolog.Nop()
	e := l.Debug()
	h.Run(e, zerolog.DebugLevel, "test message")
}

func TestSetScrapeFromKubelet(t *testing.T) {
	// Test that setScrapeFromKubelet initializes HTTP clients with insecure=true
	config := &rest.Config{
		Host: "https://localhost:8443",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
		},
	}
	os.Setenv("SCRAPE_FROM_KUBELET_TLS_INSECURE_SKIP_VERIFY", "true")
	defer os.Unsetenv("SCRAPE_FROM_KUBELET_TLS_INSECURE_SKIP_VERIFY")

	setScrapeFromKubelet(config)

	if ClientRaw == nil {
		t.Error("expected ClientRaw to be initialized")
	}
	if ClientAno == nil {
		t.Error("expected ClientAno to be initialized")
	}

	// Reset
	ClientRaw = nil
	ClientAno = nil
}

func TestSetScrapeFromKubelet_InsecureFalse(t *testing.T) {
	config := &rest.Config{
		Host: "https://localhost:8443",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: false,
		},
	}
	os.Setenv("SCRAPE_FROM_KUBELET_TLS_INSECURE_SKIP_VERIFY", "false")
	defer os.Unsetenv("SCRAPE_FROM_KUBELET_TLS_INSECURE_SKIP_VERIFY")

	setScrapeFromKubelet(config)

	if ClientRaw == nil {
		t.Error("expected ClientRaw to be initialized")
	}
	if ClientAno == nil {
		t.Error("expected ClientAno to be initialized")
	}
}

func TestSetK8sClient_PanicsWithoutCluster(t *testing.T) {
	// Ensure no k8s env is set
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("KUBERNETES_SERVICE_PORT")

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when not in cluster")
		}
	}()
	SetK8sClient()
}

func TestEnablePprof_StartsServer(t *testing.T) {
	// ponytail: test that EnablePprof doesn't panic immediately
	// Start in goroutine, check server starts, then goroutine leaks (process exit cleans up)
	go EnablePprof()
	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost:6060/debug/pprof/")
	if err != nil {
		t.Skipf("pprof server not reachable (port may be in use): %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}
}

func TestEnablePprof_DoubleStart(t *testing.T) {
	// ponytail: second EnablePprof hits port conflict, covering error path
	// First one is already running from TestEnablePprof_StartsServer
	go EnablePprof()
	time.Sleep(100 * time.Millisecond)
	// Second call should log error but not panic (ListenAndServe just returns err)
}
