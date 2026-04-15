package managementasset

import (
	"bytes"
	"fmt"
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

func TestManagementHTMLIncludesProviderDisplayNameSupport(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		"display-name",
		"Display Name",
		"Falls back to provider host when empty",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing provider display-name support: %s", needle)
		}
	}
}

func TestManagementHTMLProviderDisplayNameResolution(t *testing.T) {
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

	inlineScript := string(matches[1])
	start := strings.Index(inlineScript, "const providerDisplayHostIgnoreLabels=")
	if start < 0 {
		t.Fatalf("management asset missing provider display-name helpers")
	}
	end := strings.Index(inlineScript[start:], "function Uge(")
	if end < 0 {
		t.Fatalf("management asset missing usage statistics section after provider helpers")
	}

	snippet := inlineScript[start : start+end]
	nodeScript := fmt.Sprintf(`const ik=value=>typeof value==="string"?value.trim():"";
%s
const cases = [
	{ provider: { displayName: " Friendly Provider ", name: "Ignored Name", baseUrl: "https://api.asxs.top/v1" }, fallback: "Codex #1", want: "Friendly Provider" },
	{ provider: { name: "Configured Name", baseUrl: "https://api.openrouter.ai/v1" }, fallback: "Codex #2", want: "Configured Name" },
	{ provider: { baseUrl: "https://api.asxs.top/v1" }, fallback: "Codex #3", want: "asxs" },
	{ provider: { baseUrl: "https://api.openai.com/v1" }, fallback: "Codex #4", want: "openai" },
	{ provider: { baseUrl: "https://gateway.openai.com/v1" }, fallback: "Codex #5", want: "openai" }
];
for (const testCase of cases) {
	const got = resolveProviderDisplayName(testCase.provider, testCase.fallback);
	if (got !== testCase.want) {
		console.error(JSON.stringify({ ...testCase, got }));
		process.exit(1);
	}
}
`, snippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "provider-display-name.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write provider display-name script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management provider display-name helpers invalid: %v\n%s", errRun, stderr.String())
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
