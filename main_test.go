/*
Copyright 2026 Joseph Anthony Abbott III

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jabbott-iii/Salus/internal"
)

// useTempDatabase points SALUS_DB_PATH at an owner-only file in a temp dir so
// the misconfig check passes regardless of the process umask.
func useTempDatabase(t *testing.T) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "salus.db")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create database file: %v", err)
	}
	t.Setenv(internal.DatabasePathEnv, path)
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", t.TempDir())
	}
}

func TestRunExitCodes(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		want       int
		wantStdout string
		wantStderr string
	}{
		{name: "version", args: []string{"--version"}, want: internal.ExitCodePass, wantStdout: "salus version "},
		{name: "passing check run", args: []string{"check", "run", "--only", "misconfig"}, want: internal.ExitCodePass, wantStdout: "[PASS] misconfig"},
		{name: "group help", args: []string{"check"}, want: internal.ExitCodePass, wantStdout: "Available Commands:"},
		{name: "unknown check", args: []string{"check", "run", "--only", "does-not-exist", "--json"}, want: internal.ExitCodeError, wantStderr: `unknown check "does-not-exist"`},
		{name: "invalid threshold", args: []string{"check", "run", "--disk-warn", "95", "--json"}, want: internal.ExitCodeError, wantStderr: "--disk-warn (95) must be less than --disk-fail (90)"},
		{name: "unknown flag", args: []string{"--no-such-flag"}, want: internal.ExitCodeError, wantStderr: "unknown flag"},
		{name: "unknown subcommand flag", args: []string{"check", "run", "--no-such-flag"}, want: internal.ExitCodeError, wantStderr: "Run 'salus check run --help' for usage."},
		{name: "unknown command", args: []string{"chek"}, want: internal.ExitCodeError, wantStderr: `unknown command "chek" for "salus"`},
		{name: "unknown subcommand", args: []string{"check", "rn"}, want: internal.ExitCodeError, wantStderr: `unknown command "rn" for "salus check"; did you mean run?`},
		{name: "extra argument", args: []string{"check", "run", "extra"}, want: internal.ExitCodeError, wantStderr: `unknown command "extra" for "salus check run"`},
		{name: "invalid job id", args: []string{"jobs", "show", "abc"}, want: internal.ExitCodeError, wantStderr: `invalid job id "abc"`},
		{name: "missing job", args: []string{"jobs", "show", "999"}, want: internal.ExitCodeError, wantStderr: "scan job 999: record not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useTempDatabase(t)
			var stdout, stderr bytes.Buffer

			if got := run(tt.args, &stdout, &stderr); got != tt.want {
				t.Fatalf("run(%q) = %d, want %d\nstdout: %s\nstderr: %s", tt.args, got, tt.want, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.wantStdout)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
			if tt.want == internal.ExitCodeError {
				// Errors must never reach stdout, where they would corrupt --json output.
				if stdout.Len() != 0 {
					t.Errorf("stdout = %q, want nothing on error", stdout.String())
				}
				if n := strings.Count(stderr.String(), "Error:"); n != 1 {
					t.Errorf("stderr reports %d errors, want exactly 1:\n%s", n, stderr.String())
				}
			}
		})
	}
}

func TestRunPersistsJobsAcrossInvocations(t *testing.T) {
	useTempDatabase(t)

	var stdout, stderr bytes.Buffer
	if got := run([]string{"check", "run", "--only", "misconfig", "--quiet"}, &stdout, &stderr); got != internal.ExitCodePass {
		t.Fatalf("check run exit code = %d, want 0 (stderr: %s)", got, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("--quiet wrote %q", stdout.String())
	}

	stdout.Reset()
	if got := run([]string{"jobs", "show", "1"}, &stdout, &stderr); got != internal.ExitCodePass {
		t.Fatalf("jobs show exit code = %d, want 0 (stderr: %s)", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "[PASS] misconfig") {
		t.Errorf("jobs show output = %q, want the recorded misconfig result", stdout.String())
	}
}

func TestRunOpensDatabaseOnlyWhenNeeded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never", "salus.db")
	t.Setenv(internal.DatabasePathEnv, path)
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", t.TempDir())
	}

	for _, args := range [][]string{
		{"--version"},
		{"--help"},
		{"check"},
		{"completion", "bash"},
		{"check", "run", "--only", "misconfig", "--no-save", "--quiet"},
	} {
		var stdout, stderr bytes.Buffer
		if got := run(args, &stdout, &stderr); got != internal.ExitCodePass {
			t.Fatalf("run(%q) = %d, want 0 (stderr: %s)", args, got, stderr.String())
		}
		if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
			t.Fatalf("run(%q) created %s (stat error %v), want no database", args, filepath.Dir(path), err)
		}
	}
}

func TestRunUsesPerUserDefaultDatabase(t *testing.T) {
	t.Setenv(internal.DatabasePathEnv, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	var stdout, stderr bytes.Buffer
	if got := run([]string{"check", "list"}, &stdout, &stderr); got != internal.ExitCodePass {
		t.Fatalf("check list exit code = %d, want 0 (stderr: %s)", got, stderr.String())
	}

	want, err := internal.DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath() error = %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("database not created at the per-user default %s: %v", want, err)
	}
	if _, err := os.Stat("salus.db"); !os.IsNotExist(err) {
		t.Errorf("salus.db created in the working directory (stat error %v)", err)
	}
}

func TestRunDatabaseInitFailureIsOperationalError(t *testing.T) {
	// A directory cannot be opened as a sqlite database file.
	t.Setenv(internal.DatabasePathEnv, t.TempDir())

	var stdout, stderr bytes.Buffer
	if got := run([]string{"check", "list"}, &stdout, &stderr); got != internal.ExitCodeError {
		t.Fatalf("run() = %d, want %d (stderr: %s)", got, internal.ExitCodeError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "failed to initialize database") {
		t.Errorf("stderr = %q, want a database initialization error", stderr.String())
	}
}
