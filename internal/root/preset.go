package root

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/itunified-io/linuxctl/pkg/presets"
)

// newPresetCmd builds the `linuxctl preset` subtree: discovery + inspection
// of the embedded conventions library (plan 033).
func newPresetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "preset",
		Short: "Discover and inspect the linuxctl conventions library",
		Long: `The conventions library ships as YAML presets embedded in the linuxctl
binary. Use "linuxctl preset list" to browse what is available and
"linuxctl preset show <name>" to print a preset's content.

Bundles compose one preset per category; "linuxctl preset show <bundle> --expand"
prints all referenced presets.`,
	}
	cmd.AddCommand(newPresetListCmd())
	cmd.AddCommand(newPresetShowCmd())
	return cmd
}

func newPresetListCmd() *cobra.Command {
	var category string
	var tierStr string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List presets available to the active tier",
		RunE: func(_ *cobra.Command, _ []string) error {
			tier := presets.TierCommunity
			if tierStr != "" {
				tier = presets.Tier(tierStr)
			}
			metas := presets.List(func() presets.Tier { return tier })
			if category != "" {
				filtered := metas[:0]
				for _, m := range metas {
					if m.Category == category {
						filtered = append(filtered, m)
					}
				}
				metas = filtered
			}
			sort.Slice(metas, func(i, j int) bool {
				if metas[i].Category != metas[j].Category {
					return metas[i].Category < metas[j].Category
				}
				return metas[i].Name < metas[j].Name
			})
			tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "CATEGORY\tNAME\tTIER\tVERSION\tSOURCE")
			for _, m := range metas {
				src := m.Source
				if len(src) > 60 {
					src = src[:57] + "..."
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", m.Category, m.Name, m.Tier, m.Version, src)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "filter by category (directories|users_groups|packages|sysctl|limits|bundles)")
	cmd.Flags().StringVar(&tierStr, "tier", "", "override active tier (community|business|enterprise)")
	return cmd
}

func newPresetShowCmd() *cobra.Command {
	var expand bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a preset's YAML; --expand for bundles",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			// Try every category; preset names can repeat across categories.
			candidates := []string{"bundles", "directories", "users_groups", "packages", "sysctl", "limits"}
			var found []*presets.Preset
			for _, c := range candidates {
				if p, err := presets.ResolveCategory(c, name, nil); err == nil {
					found = append(found, p)
				}
			}
			if len(found) == 0 {
				return fmt.Errorf("preset %q not found in any category", name)
			}
			for _, p := range found {
				b, err := yaml.Marshal(p)
				if err != nil {
					return err
				}
				fmt.Printf("# %s/%s\n", p.Metadata.Category, p.Metadata.Name)
				fmt.Println(strings.TrimSpace(string(b)))
				fmt.Println("---")
				if expand && p.Kind == "Bundle" {
					children, err := presets.BundleExpand(p.Metadata.Name, nil)
					if err != nil {
						return err
					}
					cats := make([]string, 0, len(children))
					for c := range children {
						cats = append(cats, c)
					}
					sort.Strings(cats)
					for _, c := range cats {
						childName := children[c]
						cp, err := presets.ResolveCategory(c, childName, nil)
						if err != nil {
							fmt.Fprintf(os.Stderr, "warning: cannot resolve %s/%s: %v\n", c, childName, err)
							continue
						}
						cb, _ := yaml.Marshal(cp)
						fmt.Printf("# %s/%s (via bundle %s)\n", c, childName, p.Metadata.Name)
						fmt.Println(strings.TrimSpace(string(cb)))
						fmt.Println("---")
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&expand, "expand", false, "for bundles, also print each referenced preset")
	return cmd
}
