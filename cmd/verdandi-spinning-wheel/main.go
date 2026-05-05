package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/genie-cvc/verdandi/internal/spinningwheel"
	"github.com/genie-cvc/verdandi/internal/version"
)

func main() {
	dataDir := flag.String("data-dir", "", "Verdandi runtime data directory")
	addr := flag.String("addr", "127.0.0.1:8787", "HTTP listen address")
	showVersion := flag.Bool("version", false, "print version metadata")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}

	config := spinningwheel.DefaultConfig().
		WithDataDir(*dataDir).
		WithAddr(*addr).
		WithEnabled(true)

	server := spinningwheel.NewServer(config.DataDir)
	fmt.Fprintf(os.Stderr, "%s listening on http://%s\n", config.Name, config.Addr)
	if err := http.ListenAndServe(config.Addr, server.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "spinning wheel server error: %v\n", err)
		os.Exit(1)
	}
}
