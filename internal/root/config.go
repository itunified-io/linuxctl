package root

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/presets"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Validate and render linux.yaml configuration",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "validate <path>",
		Short: "Validate linux.yaml + cross-references",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			l, err := config.LoadLinux(args[0])
			if err != nil {
				return err
			}
			if err := config.Validate(l); err != nil {
				return err
			}
			fmt.Println("OK")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "render <path>",
		Short: "Render with bundles + presets expanded and secrets redacted",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return runConfigRender(c, args[0])
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use-context <name>",
		Short: "Set default context",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("config use-context: not implemented")
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print active config.yaml (redacted)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("config show: not implemented")
		},
	})
	return cmd
}

// secretRef matches ${vault:...} and ${gen:...} placeholders for redaction.
var secretRef = regexp.MustCompile(`\$\{(vault|gen):[^}]+\}`)

// runConfigRender loads a linux.yaml, expands bundle_preset + per-category
// *_preset fields into the final desired state, redacts secret references,
// and prints the resulting YAML to stdout.
func runConfigRender(c *cobra.Command, path string) error {
	l, err := config.LoadLinux(path)
	if err != nil {
		return err
	}
	// Resolve each *_preset and merge into the explicit entries.
	if l.DirectoriesPreset != "" {
		if p, err := presets.ResolveCategory("directories", l.DirectoriesPreset, nil); err == nil {
			if dirs, err := presets.DirectoriesSpec(p); err == nil {
				l.Directories = presets.MergeDirectories(l.Directories, dirs)
			}
		}
	}
	if l.UsersGroupsPreset != "" {
		if p, err := presets.ResolveCategory("users_groups", l.UsersGroupsPreset, nil); err == nil {
			if ug, err := presets.UsersGroupsSpec(p); err == nil && ug != nil {
				explicit := config.UsersGroups{}
				if l.UsersGroups != nil {
					explicit = *l.UsersGroups
				}
				merged := presets.MergeUsersGroups(explicit, *ug)
				l.UsersGroups = &merged
			}
		}
	}
	if l.PackagesPreset != "" {
		if p, err := presets.ResolveCategory("packages", l.PackagesPreset, nil); err == nil {
			if pp, err := presets.PackagesSpec(p); err == nil && pp != nil {
				explicit := config.Packages{}
				if l.Packages != nil {
					explicit = *l.Packages
				}
				merged := presets.MergePackages(explicit, *pp)
				l.Packages = &merged
			}
		}
	}
	if l.SysctlPreset != "" {
		if p, err := presets.ResolveCategory("sysctl", l.SysctlPreset, nil); err == nil {
			if entries, err := presets.SysctlSpec(p); err == nil {
				l.Sysctl = presets.MergeSysctl(l.Sysctl, entries)
			}
		}
	}
	if l.LimitsPreset != "" {
		if p, err := presets.ResolveCategory("limits", l.LimitsPreset, nil); err == nil {
			if entries, err := presets.LimitsSpec(p); err == nil {
				l.Limits = presets.MergeLimits(l.Limits, entries)
			}
		}
	}

	b, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	redacted := secretRef.ReplaceAllString(string(b), "<REDACTED>")
	out := c.OutOrStdout()
	_, err = fmt.Fprint(out, strings.TrimRight(redacted, "\n")+"\n")
	return err
}
