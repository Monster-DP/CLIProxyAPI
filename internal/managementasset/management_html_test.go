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
		"provider_first_token_latency_ms",
		"first_token_latency_ms",
		"generation_duration_ms",
		"precise_output_tps",
		"usage_stats.request_events_provider_ttft",
		"usage_stats.request_events_end_to_end_ttft",
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

func TestManagementHTMLIncludesUsageRangeRebuildFromEventPages(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		`api_key`,
		`buildUsageSnapshotFromEventItems=`,
		`loadUsageEventsForRange=`,
		`nS.getUsageEvents({range:t,limit:1000`,
		`rangeUsageLoading`,
		`rangeUsage??`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing range usage rebuild support: %s", needle)
		}
	}
}

func TestManagementHTMLRangeUsageEffectUsesStableDependencies(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	if strings.Contains(html, `}},[L,a,c?.getTime?.()??0]);`) {
		t.Fatalf("management asset range usage effect still depends on unstable usage object identity")
	}
	if !strings.Contains(html, `}},[L,!!a,c?.getTime?.()??0]);`) {
		t.Fatalf("management asset range usage effect missing stable readiness dependency")
	}
}

func TestManagementHTMLIncludesTTFTTranslationLabels(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		`request_events_provider_ttft:"供应商首包"`,
		`request_events_end_to_end_ttft:"端到端首包"`,
		`request_events_provider_ttft:"Provider TTFT"`,
		`request_events_end_to_end_ttft:"End-to-end TTFT"`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing TTFT translation label: %s", needle)
		}
	}
}

func TestManagementHTMLAlwaysShowsProviderFirstTokenColumnsAndExports(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, forbidden := range []string{
		`...hasProviderFirstToken?["provider_first_token_latency_ms"]:[]`,
		`...hasProviderFirstToken?[Y.providerFirstTokenLatencyMs??""]:[]`,
		`...hasProviderFirstToken&&$.providerFirstTokenLatencyMs!==null?{provider_first_token_latency_ms:$.providerFirstTokenLatencyMs}:{}`,
		`hasProviderFirstToken&&h.jsx("th",{children:r("usage_stats.request_events_provider_ttft")})`,
		`hasProviderFirstToken&&h.jsx("td",{className:se.durationCell,children:kc(Z.providerFirstTokenLatencyMs)})`,
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("management asset still hides provider first-token data behind conditional rendering/export: %s", forbidden)
		}
	}
}

func TestManagementHTMLFlattensProviderFirstTokenLatencyIntoUsageRows(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		`provider_first_token_latency_ms:typeof y.provider_first_token_latency_ms=="number"?y.provider_first_token_latency_ms:void 0`,
		`provider_first_token_latency_ms:typeof C.provider_first_token_latency_ms=="number"?C.provider_first_token_latency_ms:void 0`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing flattened provider first-token latency field: %s", needle)
		}
	}
}

func TestManagementHTMLShowsRequestEventTTFTWithTwoDecimalPlaces(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`children:kc\([A-Za-z_$][\w$]*\.providerFirstTokenLatencyMs,\{secondDecimals:2\}\)`),
		regexp.MustCompile(`children:kc\([A-Za-z_$][\w$]*\.firstTokenLatencyMs,\{secondDecimals:2\}\)`),
	} {
		if !pattern.MatchString(html) {
			t.Fatalf("management asset missing two-decimal TTFT rendering: %s", pattern.String())
		}
	}
}

func TestManagementHTMLTTFTFormatterKeepsTwoDecimalsWhenConfigured(t *testing.T) {
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
	start := strings.Index(inlineScript, "s$=t=>")
	if start < 0 {
		t.Fatalf("management asset missing duration formatter helpers")
	}
	end := strings.Index(inlineScript[start:], "function Ra(")
	if end < 0 {
		t.Fatalf("management asset missing duration formatter boundary")
	}

	snippet := inlineScript[start : start+end]
	nodeScript := fmt.Sprintf(`const ws = {
	resolvedLanguage: "en-US",
	language: "en-US",
	t: (_key, options = {}) => options.defaultValue ?? ""
};
let s$, i$, a$, o$, r$, Qg;
%s
const formatted = kc(1200, { secondDecimals: 2, locale: "en-US" });
if (formatted !== "1.20s") {
	console.error(JSON.stringify({ formatted }));
	process.exit(1);
}
`, snippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "ttft-formatter.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write TTFT formatter script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management TTFT formatter did not keep two decimals: %v\n%s", errRun, stderr.String())
	}
}

func TestManagementHTMLRegistersUsageChartDependencies(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	registerMatch := regexp.MustCompile(`function rme\(t,e\)\{return ib\.register\(([^)]*)\),S\.forwardRef`).FindStringSubmatch(html)
	if len(registerMatch) < 2 {
		t.Fatalf("management asset missing line-chart registration helper")
	}

	symbols := map[string]struct{}{}
	for _, part := range strings.Split(registerMatch[1], ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		symbols[trimmed] = struct{}{}
	}

	if _, ok := symbols["e"]; !ok {
		t.Fatalf("management asset line-chart helper no longer registers the chart controller")
	}

	requireRegistered := func(label string, pattern *regexp.Regexp) {
		t.Helper()

		match := pattern.FindStringSubmatch(html)
		if len(match) < 2 {
			t.Fatalf("management asset missing %s symbol", label)
		}
		if _, ok := symbols[match[1]]; !ok {
			t.Fatalf("management asset line-chart helper must register %s symbol %q", label, match[1])
		}
	}

	requireRegistered("point element", regexp.MustCompile(`Et\(([A-Za-z_$][\w$]*),"id","point"\)`))
	requireRegistered("category scale", regexp.MustCompile(`Et\(([A-Za-z_$][\w$]*),"id","category"\)`))
	requireRegistered("linear scale", regexp.MustCompile(`Et\(([A-Za-z_$][\w$]*),"id","linear"\)`))
	requireRegistered("title plugin", regexp.MustCompile(`var ([A-Za-z_$][\w$]*)=\{id:"title"`))
	requireRegistered("tooltip plugin", regexp.MustCompile(`var ([A-Za-z_$][\w$]*)=\{id:"tooltip"`))
	requireRegistered("legend plugin", regexp.MustCompile(`var ([A-Za-z_$][\w$]*)=\{id:"legend"`))
	requireRegistered("filler plugin", regexp.MustCompile(`var ([A-Za-z_$][\w$]*)=\{id:"filler"`))

	lineMatches := regexp.MustCompile(`Et\(([A-Za-z_$][\w$]*),"id","line"\)`).FindAllStringSubmatch(html, -1)
	if len(lineMatches) < 2 {
		t.Fatalf("management asset missing line controller/element definitions")
	}

	hasLineElement := false
	for _, match := range lineMatches {
		if len(match) < 2 {
			continue
		}
		if _, ok := symbols[match[1]]; ok {
			hasLineElement = true
			break
		}
	}
	if !hasLineElement {
		t.Fatalf("management asset line-chart helper must register a line element symbol")
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

func TestManagementHTMLRequestEventSourceUsesResolvedProviderDisplayName(t *testing.T) {
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
	end := strings.Index(inlineScript[start:], "const Os=\"__all__\"")
	if end < 0 {
		t.Fatalf("management asset missing request event source mapping section after provider helpers")
	}

	snippet := inlineScript[start : start+end]
	nodeScript := fmt.Sprintf(`const ik=value=>typeof value==="string"?value.trim():"";
const _i=({apiKey,prefix})=>{
	const values = [];
	if (apiKey) values.push("api:"+apiKey);
	if (prefix) values.push("prefix:"+prefix);
	return values;
};
%s
const mapping = G5({
	codexApiKeys: [{ apiKey: "codex-1", baseUrl: "https://api.asxs.top/v1" }],
	openaiCompatibility: [{ apiKeyEntries: [{ apiKey: "openai-1" }], name: "OpenRouter", baseUrl: "https://openrouter.ai/api/v1", prefix: "team-openai" }]
});
const cases = [
	{ key: "api:codex-1", want: "asxs", type: "codex" },
	{ key: "prefix:team-openai", want: "OpenRouter", type: "openai" },
	{ key: "api:openai-1", want: "OpenRouter", type: "openai" }
];
for (const testCase of cases) {
	const got = mapping.get(testCase.key);
	if (!got || got.displayName !== testCase.want || got.type !== testCase.type) {
		console.error(JSON.stringify({ ...testCase, got }));
		process.exit(1);
	}
}
`, snippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "provider-source-mapping.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write provider source mapping script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management request event source mapping invalid: %v\n%s", errRun, stderr.String())
	}
}

func TestManagementHTMLRawApiKeySourceUsesResolvedProviderDisplayName(t *testing.T) {
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
	normalizeStart := strings.Index(inlineScript, "const ri=t=>")
	if normalizeStart < 0 {
		t.Fatalf("management asset missing source key normalization helpers")
	}
	normalizeEnd := strings.Index(inlineScript[normalizeStart:], "function iO(")
	if normalizeEnd < 0 {
		t.Fatalf("management asset missing source key normalization helper boundary")
	}
	providerStart := strings.Index(inlineScript, "const providerDisplayHostIgnoreLabels=")
	if providerStart < 0 {
		t.Fatalf("management asset missing provider display-name helpers")
	}
	providerEnd := strings.Index(inlineScript[providerStart:], "const Os=\"__all__\"")
	if providerEnd < 0 {
		t.Fatalf("management asset missing request event source resolver boundary")
	}

	normalizeSnippet := inlineScript[normalizeStart : normalizeStart+normalizeEnd]
	providerSnippet := inlineScript[providerStart : providerStart+providerEnd]
	nodeScript := fmt.Sprintf(`const Ra=value=>typeof value=="string"?value:String(value??"");
const ik=value=>typeof value=="string"?value.trim():"";
%s
%s
const mapping = G5({
	codexApiKeys: [{ apiKey: "sk-live-example-123456", displayName: "ASXS Codex" }]
});
const got = $5("sk-live-example-123456", "", mapping, new Map());
if (!got || got.displayName !== "ASXS Codex" || got.type !== "codex") {
	console.error(JSON.stringify({ got }));
	process.exit(1);
}
`, normalizeSnippet, providerSnippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "raw-source-display-name.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write raw source display-name script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management raw source display-name resolution invalid: %v\n%s", errRun, stderr.String())
	}
}

func TestManagementHTMLSourceSpeedRowsMergeResolvedProviderDisplayNames(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	if !strings.Contains(html, `sourceSpeedRows=S.useMemo(()=>mergeUsageSummaryRows(sourceSummaryItems.map(oe=>normalizeSummaryItem(oe,"source")))`) {
		t.Fatalf("management asset source speed rows must merge duplicate provider display names after summary normalization")
	}
}

func TestManagementHTMLSourceSummaryMergesDuplicateProviderRowsAndDropsZeroSampleEntries(t *testing.T) {
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
	normalizeStart := strings.Index(inlineScript, "const ri=t=>")
	if normalizeStart < 0 {
		t.Fatalf("management asset missing source key normalization helpers")
	}
	normalizeEnd := strings.Index(inlineScript[normalizeStart:], "function iO(")
	if normalizeEnd < 0 {
		t.Fatalf("management asset missing source key normalization helper boundary")
	}
	providerStart := strings.Index(inlineScript, "const providerDisplayHostIgnoreLabels=")
	if providerStart < 0 {
		t.Fatalf("management asset missing provider display-name helpers")
	}
	providerEnd := strings.Index(inlineScript[providerStart:], "const Os=\"__all__\"")
	if providerEnd < 0 {
		t.Fatalf("management asset missing request event source resolver boundary")
	}
	summaryStart := strings.Index(inlineScript, "normalizeSummaryItem=")
	if summaryStart < 0 {
		t.Fatalf("management asset missing summary normalization helpers")
	}
	summaryEnd := strings.Index(inlineScript[summaryStart:], "requestRows=S.useMemo")
	if summaryEnd < 0 {
		t.Fatalf("management asset missing summary normalization boundary")
	}

	normalizeSnippet := inlineScript[normalizeStart : normalizeStart+normalizeEnd]
	providerSnippet := inlineScript[providerStart : providerStart+providerEnd]
	summarySnippet := inlineScript[summaryStart : summaryStart+summaryEnd]
	summarySnippet = strings.TrimSuffix(summarySnippet, ",")
	nodeScript := fmt.Sprintf(`const Ra=value=>typeof value=="string"?value:String(value??"");
const ik=value=>typeof value=="string"?value.trim():"";
const Ku=value=>{
	const numeric = Number(value);
	return Number.isFinite(numeric) ? numeric : 0;
};
%s
%s
const providerSourceMap = G5({
	codexApiKeys: [
		{ apiKey: "sk-closeai-1", displayName: "closeai" },
		{ apiKey: "sk-closeai-2", displayName: "closeai" }
	]
});
const authNameMap = new Map();
let normalizeSummaryItem, mergeUsageSummaryRows;
%s
const rows = mergeUsageSummaryRows([
	normalizeSummaryItem({ id: "sk-closeai-1", average_ttft_ms: 1000, average_precise_output_tps: 10, sample_count: 2 }, "source"),
	normalizeSummaryItem({ id: "sk-closeai-2", average_ttft_ms: 4000, average_precise_output_tps: 40, sample_count: 3 }, "source"),
	normalizeSummaryItem({ id: "sk-unknown-removed", average_ttft_ms: 0, average_precise_output_tps: 0, sample_count: 0 }, "source")
]);
if (!Array.isArray(rows) || rows.length !== 1) {
	console.error(JSON.stringify({ rows }));
	process.exit(1);
}
const merged = rows[0];
if (merged.label !== "closeai" || merged.sampleCount !== 5) {
	console.error(JSON.stringify({ merged }));
	process.exit(1);
}
if (Math.abs(merged.averageTtftMs - 2800) > 1e-9) {
	console.error(JSON.stringify({ merged }));
	process.exit(1);
}
if (Math.abs(merged.averagePreciseOutputTps - 28) > 1e-9) {
	console.error(JSON.stringify({ merged }));
	process.exit(1);
}
`, normalizeSnippet, providerSnippet, summarySnippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "source-summary-merge.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write source summary merge script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management source summary merge invalid: %v\n%s", errRun, stderr.String())
	}
}

func TestManagementHTMLUsageRequestEventsUseDedicatedEventEndpoints(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		`getUsageEvents:t=>Ue.get("/usage/events",{params:t,timeout:Jg})`,
		`getUsageEventSummary:t=>Ue.get("/usage/events/summary",{params:t,timeout:Jg})`,
		`getUsageEventOptions:t=>Ue.get("/usage/events/options",{params:t,timeout:Jg})`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing dedicated usage event endpoint wiring: %s", needle)
		}
	}

	if strings.Contains(html, `h.jsx(Jge,{usage:T,loading:o,`) {
		t.Fatalf("management asset still passes flattened usage snapshot into request events component")
	}

	start := strings.Index(html, "function Jge(")
	if start < 0 {
		t.Fatalf("management asset missing request events component")
	}
	end := strings.Index(html[start:], "function i_e(")
	if end < 0 {
		t.Fatalf("management asset missing usage page component after request events component")
	}

	jge := html[start : start+end]
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`nS\.getUsageEvents\(requestQuery\)`),
		regexp.MustCompile(`nS\.getUsageEventSummary\(summaryQuery\)`),
		regexp.MustCompile(`nS\.getUsageEventOptions\(optionQuery\)`),
	} {
		if !pattern.MatchString(jge) {
			t.Fatalf("management asset request events component missing dedicated endpoint fetch: %s", pattern.String())
		}
	}

	for _, forbidden := range []string{
		`Da(t).map(`,
		`IE(K,Z=>Z.source)`,
		`IE(K,Z=>Z.model)`,
	} {
		if strings.Contains(jge, forbidden) {
			t.Fatalf("management asset request events component still derives data from flattened usage summary: %s", forbidden)
		}
	}
}

func TestManagementHTMLUsagePageDefinesPersistedChartDefaults(t *testing.T) {
	assetPath := filepath.Join("..", "..", "data", "static", ManagementFileName)
	content, errRead := os.ReadFile(assetPath)
	if errRead != nil {
		t.Fatalf("read management asset: %v", errRead)
	}

	html := string(content)
	for _, needle := range []string{
		`const X5="cli-proxy-usage-chart-lines-v1"`,
		`Q5="cli-proxy-usage-time-range-v1"`,
		`tm=["all"]`,
		`q1="24h"`,
		`J5=(t,e=Y5)=>`,
		`n_e=()=>`,
		`s_e=()=>`,
		`S.useState(n_e)`,
		`S.useState(s_e)`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("management asset missing usage page persisted chart defaults: %s", needle)
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

func TestManagementHTMLSpecialCharRegexpInitializesInNode(t *testing.T) {
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
	start := strings.Index(inlineScript, `const uw=/x/.unicode!=null?"gu":"g",jxe=new RegExp(`)
	if start < 0 {
		t.Fatalf("management asset missing special-char regex initializer")
	}
	endNeedle := `65532:"object replacement"};`
	end := strings.Index(inlineScript[start:], endNeedle)
	if end < 0 {
		t.Fatalf("management asset missing special-char regex initializer boundary")
	}

	snippet := inlineScript[start : start+end+len(endNeedle)]
	nodeScript := fmt.Sprintf(`%s
console.log(String(jxe));
`, snippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "management-special-char-regexp.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write special-char regex script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management special-char regex initializer invalid: %v\n%s", errRun, stderr.String())
	}
}

func TestManagementHTMLTooltipMultilineHelperSplitsOnlyNewlines(t *testing.T) {
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
	start := strings.Index(inlineScript, `function gr(t){`)
	if start < 0 {
		t.Fatalf("management asset missing tooltip multiline helper")
	}
	end := strings.Index(inlineScript[start:], `function Ffe(`)
	if end < 0 {
		t.Fatalf("management asset missing tooltip multiline helper boundary")
	}

	snippet := inlineScript[start : start+end]
	nodeScript := fmt.Sprintf(`%s
const singleLine = gr("04-18 06:00");
if (Array.isArray(singleLine)) {
	console.error(JSON.stringify({ singleLine }));
	process.exit(1);
}
const multiLine = gr("04-18\n06:00");
if (!Array.isArray(multiLine) || multiLine.length !== 2 || multiLine[0] !== "04-18" || multiLine[1] !== "06:00") {
	console.error(JSON.stringify({ multiLine }));
	process.exit(1);
}
`, snippet)

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "management-tooltip-multiline.js")
	if errWrite := os.WriteFile(scriptPath, []byte(nodeScript), 0o600); errWrite != nil {
		t.Fatalf("write tooltip multiline helper script: %v", errWrite)
	}

	cmd := exec.Command("node", scriptPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if errRun := cmd.Run(); errRun != nil {
		t.Fatalf("management tooltip multiline helper invalid: %v\n%s", errRun, stderr.String())
	}
}
