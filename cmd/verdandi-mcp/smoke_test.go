package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMCPStdioSmokeScript(t *testing.T) {
	runSmokeScript(t, "mcp_stdio_smoke.sh")
}

func TestMCPHTTPSmokeScript(t *testing.T) {
	runSmokeScript(t, "mcp_http_smoke.sh")
}

func runSmokeScript(t *testing.T, name string) {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	script := filepath.Join(root, "scripts", name)
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("missing MCP smoke script %s: %v", name, err)
	}

	cmd := exec.Command("bash", script)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("MCP smoke script %s failed: %v\n%s", name, err, output)
	}
}
