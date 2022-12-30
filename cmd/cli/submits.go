package main

import (
	"fmt"
	"os"

	"github.com/bigredeye/notmanytask/pkg/client/notmanytask"
	"github.com/spf13/cobra"
)

func makeDumpSuccessfulSubmits() *cobra.Command {
	var task string
	var group string

	cmd := &cobra.Command{
		Use:   "submits",
		Short: "Dump submits",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dumpSuccessfulSubmits(group, task)
		},
	}

	cmd.Flags().StringVar(&group, "group", "hse", "Group name")
	cmd.Flags().StringVar(&task, "task", "", "Task name")

	return cmd
}

func dumpSuccessfulSubmits(group, task string) error {
	nmt, err := notmanytask.NewClient("https://cpp-hse.net", os.Getenv("NOTMANYTASK_TOKEN"))
	if err != nil {
		return err
	}

	users, err := nmt.LoadSuccessfulSubmits(group, task)
	if err != nil {
		return err
	}

	for _, user := range users {
		fmt.Println(user.GitlabLogin)
	}

	return nil
}
