package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	packageA = "example.com/project/a"
	packageB = "example.com/project/b"
)

func TestCheckerAcceptsInterleavedEvents(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n" + packageB + "\tTestBeta\n"
	events := strings.Join([]string{
		event("start", packageA, ""),
		event("start", packageB, ""),
		event("run", packageA, "TestAlpha"),
		event("run", packageB, "TestBeta"),
		event("run", packageA, "TestAlpha/subtest"),
		event("pass", packageB, "TestBeta"),
		event("pass", packageA, "TestAlpha/subtest"),
		event("pass", packageB, ""),
		event("pass", packageA, "TestAlpha"),
		event("pass", packageA, ""),
	}, "\n") + "\n"

	if err := check(writeManifest(t, manifest), strings.NewReader(events)); err != nil {
		t.Fatalf("check rejected valid interleaved events: %v", err)
	}
}

func TestCheckerRejectsDescendantSkip(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	events := event("run", packageA, "TestAlpha") + "\n" +
		event("skip", packageA, "TestAlpha/nested") + "\n" +
		event("pass", packageA, "TestAlpha") + "\n"
	assertCheckError(t, manifest, events, "skipped at TestAlpha/nested")
}

func TestCheckerRejectsDuplicateManifest(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n" + packageA + "\tTestAlpha\n"
	assertCheckError(t, manifest, event("run", packageA, "TestAlpha")+"\n", "duplicates")

	outOfOrder := packageB + "\tTestBeta\n" + packageA + "\tTestAlpha\n"
	assertCheckError(t, outOfOrder, event("run", packageA, "TestAlpha")+"\n", "out of byte order")

	spaceSeparated := packageA + " TestAlpha\n"
	assertCheckError(t, spaceSeparated, event("run", packageA, "TestAlpha")+"\n", "exactly one TAB")

	for name, invalid := range map[string]string{
		"no-final-lf": packageA + "\tTestAlpha",
		"blank":       packageA + "\tTestAlpha\n\n",
		"comment":     "# no comments\n",
		"invalid-utf8": string([]byte{
			0xff, '\t', 'T', 'e', 's', 't', 'A', 'l', 'p', 'h', 'a', '\n',
		}),
	} {
		t.Run(name, func(t *testing.T) {
			assertCheckError(t, invalid, event("run", packageA, "TestAlpha")+"\n", "")
		})
	}
}

func TestCheckerRejectsDuplicateOrConflictingTerminal(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	duplicate := event("run", packageA, "TestAlpha") + "\n" +
		event("pass", packageA, "TestAlpha") + "\n" +
		event("pass", packageA, "TestAlpha") + "\n"
	assertCheckError(t, manifest, duplicate, "duplicate/conflicting terminals")

	conflicting := event("run", packageA, "TestAlpha") + "\n" +
		event("pass", packageA, "TestAlpha") + "\n" +
		event("skip", packageA, "TestAlpha") + "\n"
	assertCheckError(t, manifest, conflicting, "skipped")
}

func TestCheckerRejectsMalformedOrTruncatedJSON(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	for name, events := range map[string]string{
		"malformed": "not-json\n",
		"truncated": `{"Action":"run","Package":"example.com/project/a"`,
		"blank":     "\n",
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			assertCheckError(t, manifest, events, "")
		})
	}
}

func TestCheckerRejectsMissingTest(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	events := event("start", packageA, "") + "\n" + event("pass", packageA, "") + "\n"
	assertCheckError(t, manifest, events, "did not run")
}

func TestCheckerRejectsPackageFail(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	events := event("run", packageA, "TestAlpha") + "\n" + event("fail", packageB, "") + "\n"
	assertCheckError(t, manifest, events, "package "+packageB+" failed")
}

func TestCheckerRejectsPassWithoutRun(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	assertCheckError(t, manifest, event("pass", packageA, "TestAlpha")+"\n", "pass without run")
}

func TestCheckerRejectsSkip(t *testing.T) {
	manifest := packageA + "\tTestAlpha\n"
	events := event("run", packageA, "TestAlpha") + "\n" + event("skip", packageA, "TestAlpha") + "\n"
	assertCheckError(t, manifest, events, "skipped")
}

func event(action, pkg, test string) string {
	if test == "" {
		return fmt.Sprintf(`{"Action":%q,"Package":%q}`, action, pkg)
	}
	return fmt.Sprintf(`{"Action":%q,"Package":%q,"Test":%q}`, action, pkg, test)
}

func writeManifest(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "required-tests.txt")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile returned an error: %v", err)
	}
	return path
}

func assertCheckError(t *testing.T, manifest, events, contains string) {
	t.Helper()
	err := check(writeManifest(t, manifest), strings.NewReader(events))
	if err == nil {
		t.Fatal("check unexpectedly succeeded")
	}
	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Fatalf("check error = %q, want substring %q", err, contains)
	}
}
