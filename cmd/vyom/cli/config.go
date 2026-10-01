package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/config"
	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newConfigCmd(log *logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  `View and validate the current configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, _ := cmd.Flags().GetString("subcommand")
			switch sub {
			case "show":
				return showConfig(cmd, cfg)
			case "validate":
				return validateConfig(cmd, cfg)
			default:
				return cmd.Help()
			}
		},
	}
	cmd.Flags().String("subcommand", "", "subcommand (show, validate)")
	cmd.AddCommand(newConfigShowCmd(log, cfg))
	cmd.AddCommand(newConfigValidateCmd(log, cfg))
	return cmd
}

func newConfigShowCmd(log *logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show the effective configuration (with secrets redacted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonFlag(cmd) {
				return printJSON(cfg)
			}
			// Print as YAML
			return printYAML(cfg)
		},
	}
	return cmd
}

func newConfigValidateCmd(log *logging.Logger, cfg config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.Validate(); err != nil {
				fmt.Fprintf(os.Stderr, "Configuration invalid: %v\n", err)
				return err
			}
			fmt.Fprintln(os.Stderr, "Configuration is valid.")
			return nil
		},
	}
	return cmd
}

func printYAML(v any) error {
	// Simple YAML output - just JSON for now
	return printJSON(v)
}
