package main

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"context"
	"fmt"
	"os"
)

func main() {
	err := newRootCmd().ExecuteContext(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "[-] %v\n", err)
	}
	os.Exit(cli.ExitCodeFor(err))
}
