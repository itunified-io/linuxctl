// Command docgen generates Markdown reference pages for every linuxctl
// subcommand using cobra/doc. Invoked by `make docs-cli`.
//
// Output: docs/cli/<cmd>.md plus an index aggregated into docs/cli-reference.md.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra/doc"

	"github.com/itunified-io/linuxctl/internal/root"
)

func main() {
	outDir := "docs/cli"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "mkdir:", err)
		os.Exit(1)
	}

	cmd := root.NewRootCmd(root.BuildInfo{Version: "dev", Commit: "docs", Date: "generated"})
	cmd.DisableAutoGenTag = true

	linkHandler := func(name string) string { return name }
	filePrepender := func(string) string { return "" }

	if err := doc.GenMarkdownTreeCustom(cmd, outDir, filePrepender, linkHandler); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}

	// Build aggregated docs/cli-reference.md index.
	entries, err := os.ReadDir(outDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "readdir:", err)
		os.Exit(1)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var sb strings.Builder
	sb.WriteString("# linuxctl CLI Reference\n\n")
	sb.WriteString("Auto-generated from the Cobra command tree via `cmd/docgen`. ")
	sb.WriteString("Regenerate with `make docs-cli`.\n\n")
	sb.WriteString("## Commands\n\n")
	for _, n := range names {
		title := strings.TrimSuffix(n, ".md")
		title = strings.ReplaceAll(title, "_", " ")
		sb.WriteString(fmt.Sprintf("- [`%s`](cli/%s)\n", title, n))
	}
	sb.WriteString("\n## Conventions\n\n")
	sb.WriteString("- Every subsystem manager command exposes `plan`, `apply`, `verify` verbs.\n")
	sb.WriteString("- `linuxctl apply ...` orchestrates the full 13-manager DAG.\n")
	sb.WriteString("- Persistent flags `--env`, `--host`, `--yes`, `--dry-run`, `--format` apply across the tree.\n")

	if err := os.WriteFile(filepath.Join("docs", "cli-reference.md"), []byte(sb.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write index:", err)
		os.Exit(1)
	}
	fmt.Printf("generated %d CLI pages in %s + docs/cli-reference.md\n", len(names), outDir)
}
