package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/bigredeye/notmanytask/internal/scorer"
	"github.com/bigredeye/notmanytask/pkg/client/notmanytask"
	"github.com/spf13/cobra"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
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

	p := message.NewPrinter(language.Russian)
	mapping := dumpHeader(standings.Users[0])
	for _, user := range standings.Users {
		p.Printf("%s\t%s\t%g", user.User.GitlabLogin, username(&user.User), user.FinalMark)
		iterateTasks(user, func(idx int, task *scorer.ScoredTask) {
			if mapping[task.Task] != idx {
				panic("...")
			}
			fmt.Printf("\t%d", task.Score)
		})
		fmt.Printf("\n")
	}

	return nil
}

func iterateTasks(user *scorer.UserScores, consumer func(idx int, task *scorer.ScoredTask)) {
	idx := 0
	for _, group := range user.Groups {
		for _, task := range group.Tasks {
			consumer(idx, &task)
			idx++
		}
	}
}

func dumpHeader(user *scorer.UserScores) map[string]int {
	taskmapping := make(map[string]int)

	fmt.Printf("login\tname\tmark")
	iterateTasks(user, func(idx int, task *scorer.ScoredTask) {
		taskmapping[task.Task] = idx
		fmt.Printf("\t%s", task.Task)
	})
	fmt.Printf("\n")

	return taskmapping
}

func username(user *scorer.User) string {
	first, last := user.FirstName, user.LastName
	if user.GitlabLogin == "denisrtyhb" {
		last, first = first, last
	}
	return fmt.Sprintf("%s %s", last, first)
}
