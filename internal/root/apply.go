package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/apply"
)

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply [env.yaml]",
		Short: "Orchestrate plan / apply / verify / rollback across all managers",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "plan [env.yaml]",
		Short: "Full DAG plan across all managers",
		RunE:  runApply(actionPlan),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "apply [env.yaml]",
		Short: "Apply the full DAG",
		RunE:  runApply(actionApply),
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "verify [env.yaml]",
		Short: "Verify the full DAG",
		RunE:  runApply(actionVerify),
	})
	rb := &cobra.Command{
		Use:   "rollback [env.yaml]",
		Short: "Rollback the in-memory applied set (non-persistent Phase-3 rollback)",
		RunE:  runApply(actionRollback),
	}
	rb.Flags().String("run-id", "", "UUID of the apply run to rollback (ignored in Phase 3)")
	cmd.AddCommand(rb)
	return cmd
}

const actionRollback mgrAction = 99

func runApply(action mgrAction) func(*cobra.Command, []string) error {
	return func(_ *cobra.Command, args []string) error {
		linux, err := loadLinux(envPathFromArgs(args))
		if err != nil {
			return err
		}
		sess := openSession()
		defer sess.Close()

		orch := apply.New(nil, sess, gf.dryRun).WithLinux(linux)

		ctx, cancel := deadlineCtx()
		defer cancel()

		switch action {
		case actionPlan:
			p, err := orch.Plan(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("apply plan: %d create / %d update / %d delete across %d manager(s)\n",
				p.TotalCreate, p.TotalUpdate, p.TotalDelete, len(p.ByManager))
			for name, cs := range p.ByManager {
				printChanges(name, cs)
			}
			return nil
		case actionApply:
			if !gf.yes && !gf.dryRun {
				return fmt.Errorf("refusing to apply without --yes")
			}
			r, err := orch.Apply(ctx)
			if r != nil {
				fmt.Printf("apply: applied=%d skipped=%d failed=%d\n",
					len(r.Applied), len(r.Skipped), len(r.Failed))
			}
			return err
		case actionVerify:
			v, err := orch.Verify(ctx)
			if err != nil {
				return err
			}
			if v.Matches {
				fmt.Println("apply verify: OK — no drift")
				return nil
			}
			fmt.Printf("apply verify: DRIFT in %v\n", v.InDrift)
			return nil
		case actionRollback:
			// In Phase 3, orchestrator.Rollback only rolls back in-memory
			// state from a prior Apply in the same process — which is
			// useful for tests and manual invocation from an interactive
			// session, not for cross-process recovery.
			return orch.Rollback(ctx)
		}
		return fmt.Errorf("unknown action")
	}
}
