package main

import (
	"context"
	"fmt"
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
