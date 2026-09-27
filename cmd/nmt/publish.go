package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bigredeye/notmanytask/internal/render"
)

func makePublishCommand() *cobra.Command {
	opts := render.PublishOptions{}
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Render the public course tree and push it to the template repository",
		Long: `Clones --target, renders --source into the clone (see "nmt render") and
pushes the result as one commit. Nothing is pushed when the public tree did
not change. Authentication is git's: an ssh key or a token in the URL.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := render.Publish(opts)
			if err != nil {
				return err
			}
			fmt.Print(result.Diff)
			fmt.Println(result.Summary)
			switch {
			case result.Pushed:
				fmt.Println("pushed to", opts.Target)
			case opts.DryRun:
				fmt.Println("dry run, nothing pushed")
			default:
				fmt.Println("nothing to publish")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Source, "source", ".", "checkout of the private course repository")
	cmd.Flags().StringVar(&opts.Target, "target", "", "git URL of the template repository")
	cmd.Flags().StringVar(&opts.Branch, "branch", "main", "branch of the template students fork from")
	cmd.Flags().StringVar(&opts.Message, "message", "", "commit message (default: Publish <date> <time> from <source rev>)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show the diff, do not commit or push")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}
