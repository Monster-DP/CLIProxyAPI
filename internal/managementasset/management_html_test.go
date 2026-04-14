package managementasset

import (
	"os"
	"path/filepath"
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
