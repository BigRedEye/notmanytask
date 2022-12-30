package main

import (
	"os"

	"github.com/bigredeye/notmanytask/pkg/client/notmanytask"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func makeOverrideCommand() *cobra.Command {
	var task string
	var user string
	var status string
	var score int

	cmd := &cobra.Command{
		Use:   "override",
		Short: "Override scores",
		RunE: func(cmd *cobra.Command, args []string) error {
			return overrideScores(task, user, status, score)
		},
	}

	cmd.Flags().StringVar(&task, "task", "", "Task name")
	cmd.Flags().StringVar(&user, "user", "", "User name")
	cmd.Flags().StringVar(&status, "status", "", "Task status")
	cmd.Flags().IntVar(&score, "score", 0, "Task score")

	return cmd
}

func overrideScores(task, user, status string, score int) error {
	nmt, err := notmanytask.NewClient("https://cpp-hse.net", os.Getenv("NOTMANYTASK_TOKEN"))
	if err != nil {
		return err
	}

	err = nmt.OverrideScore(user, task, status, score)
	if err != nil {
		return err
	}

	log.Info("Updated score",
		zap.String("task", task),
		zap.String("user", user),
		zap.String("status", status),
		zap.Int("score", score),
	)

	return nil
}
