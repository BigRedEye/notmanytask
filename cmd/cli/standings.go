package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/bigredeye/notmanytask/internal/scorer"
	"github.com/bigredeye/notmanytask/pkg/client/notmanytask"
	"github.com/spf13/cobra"
)

func makeDumpStandingsCommand() *cobra.Command {
	var group string
	cmd := &cobra.Command{
		Use:   "standings",
		Short: "Dump standings",
		RunE: func(cmd *cobra.Command, args []string) error {
			return dumpStandings(group)
		},
	}
	cmd.Flags().StringVar(&group, "group", "hse", "Group name")

	return cmd
}

func dumpStandings(group string) error {
	nmt, err := notmanytask.NewClient("https://cpp-hse.net", os.Getenv("NOTMANYTASK_TOKEN"))
	if err != nil {
		return err
	}

	standings, err := nmt.LoadStandings(group)
	if err != nil {
		return err
	}

	sort.Slice(standings.Users, func(i, j int) bool {
		lhs := username(&standings.Users[i].User)
		rhs := username(&standings.Users[j].User)
		return lhs < rhs
	})

	for _, user := range standings.Users {
		fmt.Printf("%s\t%.3f\n", username(&user.User), user.FinalMark)
	}

	return nil
}

func username(user *scorer.User) string {
	first, last := user.FirstName, user.LastName
	if user.GitlabLogin == "denisrtyhb" {
		last, first = first, last
	}
	return fmt.Sprintf("%s %s", last, first)
}
