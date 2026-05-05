package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/genie-cvc/verdandi/internal/mcp"
	"github.com/genie-cvc/verdandi/internal/verdandi"
	"github.com/genie-cvc/verdandi/internal/version"
)

func main() {
	dataDir := flag.String("data-dir", "", "runtime data directory")
	analyzer := flag.String("analyzer", "", "request analyzer backend: keyword, llm, or auto")
	llmEndpoint := flag.String("llm-endpoint", "", "LLM analyzer endpoint")
	llmModel := flag.String("llm-model", "", "LLM analyzer model")
	httpAddr := flag.String("http", "", "serve Streamable HTTP MCP on this address instead of stdio, for example 127.0.0.1:8080")
	mcpPath := flag.String("mcp-path", "/mcp", "Streamable HTTP MCP endpoint path")
	httpAllowedOrigins := flag.String("http-allowed-origin", "", "comma-separated HTTP Origin allowlist; localhost and loopback origins are always allowed")
	httpBearerToken := flag.String("http-bearer-token", "", "optional bearer token required for HTTP MCP requests; defaults to VERDANDI_MCP_HTTP_BEARER_TOKEN")
	httpSession := flag.Bool("http-session", false, "enable MCP-Session-Id issuance, validation, and DELETE termination for Streamable HTTP")
	showVersion := flag.Bool("version", false, "print version metadata")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}

	server := mcp.NewServer(verdandi.NewExecutor(verdandi.Options{
		DataDir:  *dataDir,
		Analyzer: *analyzer,
		LLM: verdandi.LLMAnalyzerConfig{
			Endpoint: *llmEndpoint,
			Model:    *llmModel,
		},
	}))
	if *httpAddr != "" {
		token := *httpBearerToken
		if token == "" {
			token = os.Getenv("VERDANDI_MCP_HTTP_BEARER_TOKEN")
		}
		mux := http.NewServeMux()
		mux.Handle(*mcpPath, server.HTTPHandler(mcp.HTTPOptions{
			AllowedOrigins: splitCommaList(*httpAllowedOrigins),
			BearerToken:    token,
			RequireSession: *httpSession,
		}))
		fmt.Fprintf(os.Stderr, "verdandi MCP HTTP server listening on http://%s%s\n", *httpAddr, *mcpPath)
		if err := http.ListenAndServe(*httpAddr, mux); err != nil {
			fmt.Fprintf(os.Stderr, "verdandi MCP HTTP server error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "verdandi MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	result := []string{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
