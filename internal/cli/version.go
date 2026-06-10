package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/francisguimond/kairos/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kairos version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), version.Version); err != nil {
				return fmt.Errorf("write version: %w", err)
			}
			return nil
		},
	}
}
