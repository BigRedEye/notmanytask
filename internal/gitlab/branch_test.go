package gitlab

import "testing"

func TestSubmitBranches(t *testing.T) {
	if !IsSubmitBranch("submits/intro/aplusb") || IsSubmitBranch("main") || IsSubmitBranch("tasks/x") {
		t.Fatal("IsSubmitBranch")
	}
	if ParseTaskFromBranch("submits/intro/aplusb") != "intro/aplusb" || ParseTaskFromBranch("submits/tasks/x") != "x" {
		t.Fatal("ParseTaskFromBranch")
	}
}
