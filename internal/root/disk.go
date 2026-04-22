package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/managers"
)

func newDiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disk [env.yaml|linux.yaml]",
		Short: "Manage disk state (plan / apply / verify)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml|linux.yaml]",
		Short: "Preview disk changes",
		RunE:  runManager("disk", actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml|linux.yaml]",
		Short: "Apply disk changes (DESTRUCTIVE — confirms unless --yes)",
		RunE:  runManager("disk", actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml|linux.yaml]",
		Short: "Verify disk state",
		RunE:  runManager("disk", actionVerify),
	})
	return cmd
}

type mgrAction int

const (
	actionPlan mgrAction = iota
	actionApply
	actionVerify
)

// runManager is the shared command body for single-manager plan/apply/verify.
// It loads linux.yaml, opens a session, binds it into the registered manager,
// and dispatches.
func runManager(name string, action mgrAction) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		linux, err := loadLinux(stackPathFromArgs(args))
		if err != nil {
			return err
		}
		sess := openSession()
		defer sess.Close()
		m := managers.Lookup(name)
		if m == nil {
			return fmt.Errorf("no manager registered under %q", name)
		}
		// Best-effort binding to session — managers that support it expose
		// WithSession via type assertion.
		m = bindSession(m, sess)
		desired := desiredFor(linux, name)

		ctx, cancel := deadlineCtx()
		defer cancel()

		switch action {
		case actionPlan:
			changes, err := m.Plan(ctx, desired, nil)
			if err != nil {
				return err
			}
			printChanges(name, changes)
			return nil
		case actionApply:
			changes, err := m.Plan(ctx, desired, nil)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				fmt.Printf("%s: nothing to apply\n", name)
				return nil
			}
			printChanges(name, changes)
			if !gf.yes && !gf.dryRun {
				fmt.Printf("Apply %d change(s) to %s? (use --yes to skip)\n", len(changes), sess.Host())
				return fmt.Errorf("refusing to apply without --yes")
			}
			res, err := m.Apply(ctx, changes, gf.dryRun)
			fmt.Printf("%s: applied=%d skipped=%d failed=%d duration=%s\n",
				name, len(res.Applied), len(res.Skipped), len(res.Failed), res.Duration)
			return err
		case actionVerify:
			v, err := m.Verify(ctx, desired)
			if err != nil {
				return err
			}
			if v.OK {
				fmt.Printf("%s: OK (no drift)\n", name)
				return nil
			}
			fmt.Printf("%s: DRIFT — %d change(s) pending\n", name, len(v.Drift))
			printChanges(name, v.Drift)
			return nil
		}
		return fmt.Errorf("unreachable")
	}
}
