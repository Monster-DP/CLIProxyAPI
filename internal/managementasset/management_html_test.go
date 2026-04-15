package managementasset

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestManagementHTMLIncludesFirstByteTimeoutConfig(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	if !strings.Contains(html, "first-byte-timeout-ms") {
		t.Fatalf("management asset missing first-byte-timeout-ms support")
	}
	if !strings.Contains(html, "first_byte_timeout_ms") {
		t.Fatalf("management asset missing first-byte-timeout field wiring")
	}
}

func TestManagementHTMLIncludesTokenSpeedUsageMetrics(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		"first_token_latency_ms",
		"generation_duration_ms",
		"precise_output_tps",
		"usage_stats.request_events_ttft",
		"usage_stats.request_events_precise_tps",
		"usage_stats.request_events_precise_tps_hint",
		"usage_stats.speed_by_source",
		"usage_stats.speed_by_model",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing token speed support: %s", needle)
		}
	}
}

func TestManagementHTMLInlineScriptParsesWithNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}

	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	matches := regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`).FindSubmatch(content)
	if len(matches) < 2 {
		t.Fatalf("management asset missing inline script")
	}

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "management-inline.js")
	if errWrite := os.WriteFile(scriptPath, matches[1], 0o600); errWrite != nil {
		t.Fatalf("write inline script: %v", errWrite)
	}

	cmd := exec.Command("node", "--check", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management inline script syntax invalid: %v\n%s", errRun, stderr.String())
	}
}
