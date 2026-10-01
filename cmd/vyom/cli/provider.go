package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kaushalacts/vyom/internal/logging"
	"github.com/kaushalacts/vyom/internal/repository"
	"github.com/kaushalacts/vyom/pkg/models"
	"github.com/kaushalacts/vyom/internal/providererr"
)

func newProviderCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage provider configurations",
		Long: `Manage CDN and storage provider configurations.

Providers are stored in the database. Secrets are never stored; only the names
of environment variables that hold the secrets.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newProviderAddCmd(log, db))
	cmd.AddCommand(newProviderListCmd(log, db))
	cmd.AddCommand(newProviderRemoveCmd(log, db))
	cmd.AddCommand(newProviderShowCmd(log, db))
	return cmd
}

func newProviderAddCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	var (
		kind         string
		providerType string
		name         string
		options      []string
		credentials  []string
	)
	cmd := &cobra.Command{
		Use:   "add [cdn|storage] <type>",
		Short: "Add a provider configuration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			kind = args[0]
			providerType = args[1]

			if kind != "cdn" && kind != "storage" {
				return providererr.Configuration("cli", "provider add", "kind must be 'cdn' or 'storage'")
			}

			var kindEnum models.ProviderKind
			if kind == "cdn" {
				kindEnum = models.KindCDN
			} else {
				kindEnum = models.KindStorage
			}

			typeEnum := models.ProviderType(providerType)
			if !typeEnum.Valid() {
				return providererr.Configuration("cli", "provider add", fmt.Sprintf("unknown provider type %q", providerType))
			}
			if typeEnum.Kind() != kindEnum {
				return providererr.Configuration("cli", "provider add",
					fmt.Sprintf("type %s is not a %s provider", providerType, kind))
			}

			opts := parseKeyValues(options)
			creds := parseSecretRefs(credentials)

			p := models.ProviderConfig{
				Kind:        kindEnum,
				Type:        typeEnum,
				Name:        name,
				Enabled:     true,
				Options:     opts,
				Credentials: creds,
				Status:      models.ProviderUnconfigured,
			}

			if err := db.UpsertProvider(ctx, &p); err != nil {
				return providererr.Wrap(err, "cli", "provider add", providererr.KindInternal)
			}
			fmt.Fprintf(os.Stderr, "Provider added: %s (%s:%s)\n", p.Name, kind, providerType)
			fmt.Fprintf(os.Stderr, "ID: %s\n", p.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "provider name (required)")
	cmd.Flags().StringArrayVar(&options, "option", nil, "option key=value (repeatable)")
	cmd.Flags().StringArrayVar(&credentials, "credential", nil, "credential env var (repeatable): client_token_env=VAR, client_secret_env=VAR, access_token_env=VAR, account_key_env=VAR")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newProviderListCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List provider configurations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			providers, err := db.ListProviders(ctx)
			if err != nil {
				return providererr.Wrap(err, "cli", "provider list", providererr.KindInternal)
			}
			if jsonFlag(cmd) {
				return printJSON(providers)
			}
			if len(providers) == 0 {
				fmt.Fprintln(os.Stderr, "No providers configured.")
				return nil
			}
			for _, p := range providers {
				statusIcon := "✓"
				if !p.Status.OK() {
					statusIcon = "✗"
				}
				fmt.Fprintf(os.Stderr, "%s %s (%s:%s) [%s] - %s\n",
					statusIcon, p.Name, p.Kind, p.Type, p.ID, p.Status)
			}
			return nil
		},
	}
	return cmd
}

func newProviderRemoveCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a provider configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id := args[0]
			if err := db.DeleteProvider(ctx, id); err != nil {
				return providererr.Wrap(err, "cli", "provider remove", providererr.KindInternal)
			}
			fmt.Fprintf(os.Stderr, "Provider removed: %s\n", id)
			return nil
		},
	}
	return cmd
}

func newProviderShowCmd(log *logging.Logger, db repository.DB) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a provider configuration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			id := args[0]
			p, err := db.GetProvider(ctx, id)
			if err != nil {
				return providererr.Wrap(err, "cli", "provider show", providererr.KindInternal)
			}
			if jsonFlag(cmd) {
				return printJSON(p)
			}
			fmt.Fprintf(os.Stderr, "ID: %s\n", p.ID)
			fmt.Fprintf(os.Stderr, "Kind: %s\n", p.Kind)
			fmt.Fprintf(os.Stderr, "Type: %s\n", p.Type)
			fmt.Fprintf(os.Stderr, "Name: %s\n", p.Name)
			fmt.Fprintf(os.Stderr, "Enabled: %v\n", p.Enabled)
			fmt.Fprintf(os.Stderr, "Status: %s\n", p.Status)
			fmt.Fprintf(os.Stderr, "Options: %v\n", p.Options)
			fmt.Fprintf(os.Stderr, "Credentials (env refs): %v\n", p.Credentials)
			return nil
		},
	}
	return cmd
}

func parseKeyValues(kvs []string) map[string]string {
	m := make(map[string]string)
	for _, kv := range kvs {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

func parseSecretRefs(refs []string) models.SecretRefs {
	var s models.SecretRefs
	for _, ref := range refs {
		k, v, _ := strings.Cut(ref, "=")
		switch k {
		case "client_token_env":
			s.ClientTokenEnv = v
		case "client_secret_env":
			s.ClientSecretEnv = v
		case "access_token_env":
			s.AccessTokenEnv = v
		case "account_key_env":
			s.AccountKeyEnv = v
		case "subscription_env":
			s.SubscriptionEnv = v
		case "api_key_env":
			s.APIKeyEnv = v
		}
	}
	return s
}
