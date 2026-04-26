package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/genie-cvc/verdandi/internal/mcp"
	"github.com/genie-cvc/verdandi/internal/verdandi"
)

func main() {
	dataDir := flag.String("data-dir", "", "runtime data directory")
	analyzer := flag.String("analyzer", "", "request analyzer backend: keyword, llm, or auto")
	llmEndpoint := flag.String("llm-endpoint", "", "LLM analyzer endpoint")
	llmModel := flag.String("llm-model", "", "LLM analyzer model")
	flag.Parse()

	server := mcp.NewServer(verdandi.NewExecutor(verdandi.Options{
		DataDir:  *dataDir,
		Analyzer: *analyzer,
		LLM: verdandi.LLMAnalyzerConfig{
			Endpoint: *llmEndpoint,
			Model:    *llmModel,
		},
	}))
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "verdandi MCP server error: %v\n", err)
		os.Exit(1)
	}
}
