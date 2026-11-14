package cli

import "github.com/spf13/cobra"

// NewRoot builds the full command tree. Package main sets it: it is the only package that sees every command,
// and the few commands that describe the whole tree (the docs generator) reach it through here.
var NewRoot func() *cobra.Command
