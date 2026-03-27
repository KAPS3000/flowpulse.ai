package main

import (
	"fmt"
	"os"

	"github.com/flowpulse/flowpulse/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("fpctl %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
	case "flows":
		fmt.Println("flow listing not yet implemented")
	case "topology":
		fmt.Println("topology view not yet implemented")
	case "metrics":
		fmt.Println("metrics view not yet implemented")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: fpctl <command>

Commands:
  version    Show version information
  flows      List and filter flows
  topology   Show cluster topology
  metrics    Show training metrics
`)
}
