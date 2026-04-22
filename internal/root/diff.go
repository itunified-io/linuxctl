package root

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/itunified-io/linuxctl/pkg/apply"
)

func newDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff [env.yaml]",
		Short: "Read-only drift report across all managers",
		RunE: func(_ *cobra.Command, args []string) error {
			linux, err := loadLinux(stackPathFromArgs(args))
			if err != nil {
				return err
			}
			sess := openSession()
			defer sess.Close()
			orch := apply.New(nil, sess, true).WithLinux(linux)
			ctx, cancel := deadlineCtx()
			defer cancel()
			d, err := orch.Diff(ctx)
			if err != nil {
				return err
			}
			if d.Empty {
				fmt.Println("diff: no drift")
				return nil
			}
			for name, cs := range d.ByManager {
				printChanges(name, cs)
			}
			return nil
		},
	}
}
