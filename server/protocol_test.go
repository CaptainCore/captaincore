package server

import (
	"reflect"
	"testing"
)

// New protocol: argv persisted as JSON reconstructs verbatim, spaces preserved.
func TestBuildExecFromArgsJSON(t *testing.T) {
	task := Task{ArgsJSON: `["snapshot","generate","site@prov","--notes=two words","--rollback=2026-01-01 10:00"]`}
	head, args := buildExec(task, "42")
	if head != "captaincore" {
		t.Fatalf("head = %q, want captaincore", head)
	}
	want := []string{"--captain-id=42", "snapshot", "generate", "site@prov", "--notes=two words", "--rollback=2026-01-01 10:00"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v\nwant   %#v", args, want)
	}
}

// New protocol: in-memory Args (fresh request, before persistence) also work.
func TestBuildExecFromArgsInMemory(t *testing.T) {
	task := Task{Args: []string{"backup", "download", "site", "--payload=tok"}}
	head, args := buildExec(task, "7")
	want := []string{"--captain-id=7", "backup", "download", "site", "--payload=tok"}
	if head != "captaincore" || !reflect.DeepEqual(args, want) {
		t.Fatalf("head=%q args=%#v", head, args)
	}
}

// Legacy protocol: command string still tokenizes, with the historical
// --name= double-quote strip and --link= quote retention preserved.
func TestBuildExecLegacyString(t *testing.T) {
	task := Task{Command: `deactivate site --name="John Doe" --link="https://x.com"`}
	head, args := buildExec(task, "1")
	want := []string{"--captain-id=1", "deactivate", "site", "--name=John Doe", `--link="https://x.com"`}
	if head != "captaincore" || !reflect.DeepEqual(args, want) {
		t.Fatalf("head=%q\nargs=%#v\nwant=%#v", head, args, want)
	}
}

func TestParseCommandStringEmpty(t *testing.T) {
	head, args := parseCommandString("")
	if head != "" || args != nil {
		t.Fatalf("expected empty head/args, got %q %#v", head, args)
	}
}

// prepareArgvTask bakes payload into argv + persists ArgsJSON; the display
// Command is human-readable.
func TestPrepareArgvTaskPersistsAndDisplays(t *testing.T) {
	task := Task{Token: "tkn", Args: []string{"snapshot", "generate", "site"}}
	prepareArgvTask(&task)
	if task.ArgsJSON == "" {
		t.Fatal("ArgsJSON not set")
	}
	if task.Command != "captaincore snapshot generate site" {
		t.Fatalf("display Command = %q", task.Command)
	}
	// A legacy (no Args) task must be left untouched.
	legacy := Task{Command: "backup site"}
	prepareArgvTask(&legacy)
	if legacy.ArgsJSON != "" || legacy.Command != "backup site" {
		t.Fatalf("legacy task mutated: %+v", legacy)
	}
}
