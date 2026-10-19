package main

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
)

const version = "0.1.0-alpha"

func main() {
	err := newRootCmd().ExecuteContext(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
	}
	os.Exit(exitCodeFor(err))
}

// legacyCommands returns the commands whose handlers still parse their own arguments (DEN-950).
func legacyCommands() []*cobra.Command {
	return []*cobra.Command{
	}
}
