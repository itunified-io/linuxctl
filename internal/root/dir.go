package root

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/config"
	"github.com/itunified-io/linuxctl/pkg/managers"
)

func newDirCmd() *cobra.Command {
	var manifest string

	cmd := &cobra.Command{
		Use:   "dir",
		Short: "Manage dir state (plan / apply / verify)",
	}
	cmd.PersistentFlags().StringVarP(&manifest, "file", "f", "linux.yaml", "Path to linux.yaml or env.yaml manifest")

	cmd.AddCommand(&cobra.Command{
		Use:   "plan",
		Short: "Preview dir changes against the desired state",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDirCmd("plan", manifest)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply",
		Short: "Apply dir changes to reach the desired state",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDirCmd("apply", manifest)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify",
		Short: "Verify observed dir state matches desired state",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runDirCmd("verify", manifest)
		},
	})
	return cmd
}

// runDirCmd is the shared plan/apply/verify entrypoint for `linuxctl dir`.
func runDirCmd(op, manifest string) error {
	l, err := loadLinux(manifest)
	if err != nil {
		return fmt.Errorf("load %s: %w", manifest, err)
	}
	if err := config.Validate(l); err != nil {
		return fmt.Errorf("validate %s: %w", manifest, err)
	}

	sess := openSession()
	defer sess.Close()

	dm := managers.NewDirManager().WithSession(sess)
	ctx := context.Background()

	switch op {
	case "plan":
		changes, err := dm.Plan(ctx, l, nil)
		if err != nil {
			return err
		}
		printChanges("dir", changes)
		return nil
	case "apply":
		changes, err := dm.Plan(ctx, l, nil)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			fmt.Println("dir: no drift — nothing to apply")
			return nil
		}
		res, err := dm.Apply(ctx, changes, gf.dryRun)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "dir: applied=%d skipped=%d failed=%d\n",
			len(res.Applied), len(res.Skipped), len(res.Failed))
		for _, f := range res.Failed {
			fmt.Fprintf(os.Stderr, "  FAILED %s: %v\n", f.Change.Target, f.Err)
		}
		if len(res.Failed) > 0 {
			return fmt.Errorf("%d dir change(s) failed", len(res.Failed))
		}
		return nil
	case "verify":
		vr, err := dm.Verify(ctx, l)
		if err != nil {
			return err
		}
		if vr.OK {
			fmt.Println("dir: OK — no drift")
			return nil
		}
		fmt.Fprintf(os.Stdout, "dir: DRIFT — %d change(s) would be applied\n", len(vr.Drift))
		printChanges("dir", vr.Drift)
		return fmt.Errorf("drift detected")
	}
	return fmt.Errorf("unknown op %q", op)
}
