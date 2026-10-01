package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/version"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := version.Get()
			if jsonFlag(cmd) {
				return printJSON(info)
			}
			fmt.Println(info.String())
			return nil
		},
	}
	return cmd
}
