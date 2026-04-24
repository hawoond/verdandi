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
	flag.Parse()

	server := mcp.NewServer(verdandi.NewExecutor(verdandi.Options{DataDir: *dataDir}))
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "verdandi MCP server error: %v\n", err)
		os.Exit(1)
	}
}
