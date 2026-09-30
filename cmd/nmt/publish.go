package main

import (
	"fmt"
	"io"
	"os"

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
			if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
				opts.Color = true
			}
			result, err := render.Publish(opts)
			if err != nil {
				return err
			}
			writePublishResult(os.Stdout, os.Stderr, opts, result)
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.Source, "source", ".", "checkout of the private course repository")
	cmd.Flags().StringVar(&opts.Target, "target", "", "git URL of the template repository")
	cmd.Flags().StringVar(&opts.Branch, "branch", "main", "branch of the template students fork from")
	cmd.Flags().StringVar(&opts.Message, "message", "", "commit message (default: Publish <date> <time> from <source rev>)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "show the diff, do not commit or push")
	cmd.Flags().BoolVar(&opts.Patch, "diff", false, "print the full diff to stdout (status goes to stderr)")
	_ = cmd.MarkFlagRequired("target")
	return cmd
}

// writePublishResult prints the patch, if any, to out and the stat, summary
// and status to status, so that `nmt publish --dry-run --diff > publish.patch`
// stays a clean patch.
func writePublishResult(out, status io.Writer, opts render.PublishOptions, result *render.PublishResult) {
	fmt.Fprint(out, result.Patch)
	fmt.Fprint(status, result.Diff)
	fmt.Fprintln(status, result.Summary)
	switch {
	case result.Pushed:
		fmt.Fprintln(status, "pushed to", opts.Target)
	case opts.DryRun:
		fmt.Fprintln(status, "dry run, nothing pushed")
	default:
		fmt.Fprintln(status, "nothing to publish")
	}
}
