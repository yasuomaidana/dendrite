package bridge

import (
	"Dendrite/cmd/dendrite/internal/cli"
	"Dendrite/pkg/rea/analysis"
	"fmt"

	"github.com/spf13/cobra"
)

// reaExportCommand is `dendrite retro export`: the structure of a finished analysis as source code in another language.
func reaExportCommand() *cobra.Command {
	var lang, output string
	cmd := &cobra.Command{
		Use:     "export <id>",
		Short:   "Write the structure of a finished analysis as safe Rust structs under --output",
		Example: "dendrite retro export pe-1a2b3c4d --target-language rust --output ./generated",
		Args:    cli.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if lang != "rust" {
				return fmt.Errorf("unsupported --target-language %q: only rust is available", lang)
			}
			return withReaService(func(s *analysis.Service) error {
				path, err := s.ExportRust(args[0], output)
				if err != nil {
					return err
				}
				if cli.Globals.JSON {
					return cli.WriteJSON(cmd.OutOrStdout(), map[string]string{"id": args[0], "language": lang, "file": path})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[✓] Analysis %s exported as Rust: %s\n", args[0], path)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&lang, "target-language", "rust", "Language of the generated code (rust)")
	cmd.Flags().StringVar(&output, "output", "", "Directory to write the generated file into")
	return cmd
}
