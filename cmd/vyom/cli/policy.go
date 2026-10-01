package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/policy"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newPolicyCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage optimization policies",
		Long:  `Manage optimization policies. Policies define size thresholds, quality gates, and target formats.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPolicyListCmd(log, db))
	cmd.AddCommand(newPolicyShowCmd(log, db))
	cmd.AddCommand(newPolicySetCmd(log, db))
	cmd.AddCommand(newPolicyValidateCmd(log, db))
	return cmd
}

func newPolicyListCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List optimization policies",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			policies, err := db.ListPolicies(ctx)
			if err != nil {
				return providererr.Wrap(err, "cli", "policy list", providererr.KindInternal)
			}
			if jsonFlag(cmd) {
				return printJSON(policies)
			}
			if len(policies) == 0 {
				fmt.Fprintln(os.Stderr, "No policies configured.")
				return nil
			}
			for _, p := range policies {
				fmt.Fprintf(os.Stderr, "%s (v%s) - %s\n", p.Name, p.Version, p.Description)
			}
			return nil
		},
	}
	return cmd
}

func newPolicyShowCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show an optimization policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			p, err := db.GetPolicy(ctx, name)
			if err != nil {
				return providererr.Wrap(err, "cli", "policy show", providererr.KindInternal)
			}
			if jsonFlag(cmd) {
				return printJSON(p)
			}
			fmt.Fprintf(os.Stderr, "Name: %s\n", p.Name)
			fmt.Fprintf(os.Stderr, "Version: %s\n", p.Version)
			fmt.Fprintf(os.Stderr, "Description: %s\n", p.Description)
			fmt.Fprintf(os.Stderr, "Images: %+v\n", p.Images)
			fmt.Fprintf(os.Stderr, "Video: %+v\n", p.Video)
			fmt.Fprintf(os.Stderr, "Pricing: %+v\n", p.Pricing)
			return nil
		},
	}
	return cmd
}

func newPolicySetCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a policy from a YAML file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			file, _ := cmd.Flags().GetString("file")
			if file == "" {
				return providererr.Configuration("cli", "policy set", "--file is required")
			}
			p, err := policy.LoadFromFile(file)
			if err != nil {
				return providererr.Wrap(err, "cli", "policy set:load", providererr.KindConfiguration)
			}
			p.Name = name
			if err := db.UpsertPolicy(ctx, p); err != nil {
				return providererr.Wrap(err, "cli", "policy set:upsert", providererr.KindInternal)
			}
			fmt.Fprintf(os.Stderr, "Policy %q set (version %s)\n", name, p.Version)
			return nil
		},
	}
	cmd.Flags().String("file", "", "policy YAML file (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

func newPolicyValidateCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <name>",
		Short: "Validate a policy",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			name := args[0]
			p, err := db.GetPolicy(ctx, name)
			if err != nil {
				return providererr.Wrap(err, "cli", "policy validate:get", providererr.KindInternal)
			}
			if err := policy.Validate(*p); err != nil {
				fmt.Fprintf(os.Stderr, "Policy %q invalid: %v\n", name, err)
				return err
			}
			fmt.Fprintf(os.Stderr, "Policy %q is valid.\n", name)
			return nil
		},
	}
	return cmd
}
