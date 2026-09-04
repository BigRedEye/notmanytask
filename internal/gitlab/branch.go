package gitlab

import "strings"

const (
	branchPrefix = "submits/"
	tasksPrefix  = "tasks/"
)

func ParseTaskFromBranch(task string) string {
	return strings.TrimPrefix(strings.TrimPrefix(task, branchPrefix), tasksPrefix)
}

func IsSubmitBranch(name string) bool {
	return strings.HasPrefix(name, branchPrefix)
}
