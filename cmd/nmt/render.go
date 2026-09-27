package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bigredeye/notmanytask/internal/render"
)

func makeRenderCommand() *cobra.Command {
	var source, out string
	cmd := &cobra.Command{
		Use:   "render",
		Short: "Build the public tree of a course from its private source",
		Long: `Reads course.yaml in --source, exports the tasks listed in its deadlines
files without private and solution directories plus the files export.include
names, and makes --out match that tree exactly (a .git directory in --out is
left alone). Committing and pushing the result is left to git.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			summary, err := render.Render(source, out)
			if err != nil {
				return err
			}
			for _, rel := range summary.Added {
				fmt.Println("A", rel)
			}
			for _, rel := range summary.Updated {
				fmt.Println("M", rel)
			}
			for _, rel := range summary.Deleted {
				fmt.Println("D", rel)
			}
			fmt.Println(summary)
			return nil
		},
	}
	cmd.Flags().StringVar(&source, "source", ".", "checkout of the private course repository")
	cmd.Flags().StringVar(&out, "out", "", "directory to make match the public tree, e.g. a checkout of the template")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
