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

package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// openerFor returns a DatabaseOpener that always yields db.
func openerFor(db *Database) DatabaseOpener {
	return func() (*Database, error) { return db, nil }
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestCheckListCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	expectedErr := errors.New("write failed")

	cmd := newCheckListCmd(openerFor(db))
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}

func TestJobsListCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	_, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass, Duration: time.Millisecond}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	expectedErr := errors.New("write failed")

	cmd := newJobsListCmd(openerFor(db))
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)

	err = cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}

func TestJobsShowCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	job, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass, Duration: time.Millisecond}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	expectedErr := errors.New("write failed")

	cmd := newJobsShowCmd(openerFor(db))
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{strconv.Itoa(int(job.ID))})

	err = cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}

// executeCheckRun runs `check run` with args and returns stdout, stderr, and the error.
func executeCheckRun(t *testing.T, db *Database, args ...string) (string, string, error) {
	t.Helper()

	cmd := newCheckRunCmd(openerFor(db))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func countScanJobs(t *testing.T, db *Database) int {
	t.Helper()

	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	return len(jobs)
}

func TestCheckRunPassRecordsJob(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	stdout, _, err := executeCheckRun(t, db, "--only", keyMisconfig)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if !strings.Contains(stdout, "[PASS] misconfig") {
		t.Errorf("output = %q, want a PASS line for misconfig", stdout)
	}

	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("recorded %d jobs, want 1", len(jobs))
	}
	if jobs[0].FinishedAt == nil || jobs[0].FinishedAt.Before(jobs[0].StartedAt) {
		t.Errorf("job times = started %v, finished %v; want finished at or after started", jobs[0].StartedAt, jobs[0].FinishedAt)
	}
}

func TestCheckRunNoSaveSkipsPersistence(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	if _, _, err := executeCheckRun(t, db, "--only", keyMisconfig, "--no-save"); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if n := countScanJobs(t, db); n != 0 {
		t.Errorf("recorded %d jobs with --no-save, want 0", n)
	}
}

func TestCheckRunJSONOutput(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	stdout, _, err := executeCheckRun(t, db, "--only", keyMisconfig, "--json")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var outcomes []CheckOutcome
	if err := json.Unmarshal([]byte(stdout), &outcomes); err != nil {
		t.Fatalf("output is not a JSON outcome array: %v\n%s", err, stdout)
	}
	if len(outcomes) != 1 || outcomes[0].Key != keyMisconfig || outcomes[0].Status != StatusPass {
		t.Errorf("outcomes = %+v, want one PASS misconfig outcome", outcomes)
	}
}

func TestCheckRunQuietSuppressesAllOutput(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	for _, args := range [][]string{{"--quiet"}, {"--quiet", "--json"}} {
		stdout, stderr, err := executeCheckRun(t, db, append([]string{"--only", keyMisconfig}, args...)...)
		if err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
		if stdout != "" || stderr != "" {
			t.Errorf("Execute(%v) wrote stdout %q, stderr %q; want no output", args, stdout, stderr)
		}
	}
}

func TestCheckRunFailOnlyHidesPassingChecks(t *testing.T) {
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	stdout, _, err := executeCheckRun(t, db, "--only", keyMisconfig, "--fail-only")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(stdout, "[PASS]") {
		t.Errorf("--fail-only output contains a PASS line: %q", stdout)
	}
}

func TestCheckRunWarnReturnsExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the misconfig permission warning does not apply on Windows")
	}
	isolateMisconfigEnv(t)
	db := newSeededTestDatabase(t)

	dbFile := filepath.Join(t.TempDir(), "writable.db")
	if err := os.WriteFile(dbFile, nil, 0o600); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := os.Chmod(dbFile, 0o666); err != nil {
		t.Fatalf("chmod file: %v", err)
	}
	t.Setenv(DatabasePathEnv, dbFile)

	stdout, stderr, err := executeCheckRun(t, db, "--only", keyMisconfig)
	var status *ExitStatusError
	if !errors.As(err, &status) || status.Code != ExitCodeWarn {
		t.Fatalf("Execute() error = %v, want ExitStatusError with code %d", err, ExitCodeWarn)
	}
	if !strings.Contains(stdout, "[WARN] misconfig") {
		t.Errorf("output = %q, want a WARN line for misconfig", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing (no error or usage text for a WARN result)", stderr)
	}
	if n := countScanJobs(t, db); n != 1 {
		t.Errorf("recorded %d jobs, want 1", n)
	}
}

func TestCheckRunFailReturnsExitStatus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the disk-space check can only FAIL on Linux")
	}
	db := newSeededTestDatabase(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, _, err := executeCheckRun(t, db, "--only", keyDiskSpace, "--disk-path", missing)
	if got := ExitCode(err); got != ExitCodeFail {
		t.Fatalf("ExitCode(%v) = %d, want %d", err, got, ExitCodeFail)
	}
}

func TestCheckRunUnknownCheckIsOperationalError(t *testing.T) {
	db := newSeededTestDatabase(t)

	_, _, err := executeCheckRun(t, db, "--only", "does-not-exist")
	if err == nil {
		t.Fatal("Execute() error = nil, want unknown check error")
	}
	if got := ExitCode(err); got != ExitCodeError {
		t.Errorf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
	}
	if n := countScanJobs(t, db); n != 0 {
		t.Errorf("recorded %d jobs for a failed run, want 0", n)
	}
}

func TestRecordScanKeepsStartTime(t *testing.T) {
	db := newSeededTestDatabase(t)
	startedAt := time.Now().Add(-2 * time.Second)

	job, err := RecordScan(db, startedAt, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	// Read the job back so the check covers what SQLite actually stores.
	stored, _, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}
	if diff := stored.StartedAt.Sub(startedAt); diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("stored StartedAt = %v, want %v", stored.StartedAt, startedAt)
	}
	if stored.FinishedAt == nil || stored.FinishedAt.Sub(stored.StartedAt) < 2*time.Second {
		t.Errorf("stored FinishedAt = %v, want at least 2s after StartedAt %v", stored.FinishedAt, stored.StartedAt)
	}
}

func TestGroupCommandsRejectUnknownSubcommands(t *testing.T) {
	db := newSeededTestDatabase(t)

	for _, group := range []*cobra.Command{newCheckCmd(openerFor(db)), newJobsCmd(openerFor(db))} {
		t.Run(group.Name(), func(t *testing.T) {
			// Attach to a parent: a command executed as the root gets Cobra's
			// own subcommand handling instead of runGroup.
			root := &cobra.Command{Use: "salus", SilenceErrors: true, SilenceUsage: true}
			root.AddCommand(group)
			var stdout bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(io.Discard)

			root.SetArgs([]string{group.Name(), "lst"})
			err := root.Execute()
			want := `unknown command "lst" for "salus ` + group.Name() + `"; did you mean list?`
			if err == nil || err.Error() != want {
				t.Fatalf("Execute(%s lst) error = %v, want %q", group.Name(), err, want)
			}
			if got := ExitCode(err); got != ExitCodeError {
				t.Errorf("ExitCode = %d, want %d", got, ExitCodeError)
			}

			stdout.Reset()
			root.SetArgs([]string{group.Name()})
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(%s) error = %v, want help", group.Name(), err)
			}
			if !strings.Contains(stdout.String(), "Available Commands:") {
				t.Errorf("help output = %q, want the subcommand list", stdout.String())
			}
		})
	}
}

func TestCheckRunNoSaveNeverOpensDatabase(t *testing.T) {
	isolateMisconfigEnv(t)
	openDB := func() (*Database, error) {
		t.Error("check run --no-save opened the database")
		return nil, errors.New("unexpected open")
	}

	cmd := newCheckRunCmd(openDB)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--only", keyMisconfig, "--no-save"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCommandsReportDatabaseOpenErrors(t *testing.T) {
	openErr := errors.New("disk on fire")
	openDB := func() (*Database, error) { return nil, openErr }

	commands := map[string]*cobra.Command{
		"check list": newCheckListCmd(openDB),
		"check run":  newCheckRunCmd(openDB),
		"jobs list":  newJobsListCmd(openDB),
		"jobs show":  newJobsShowCmd(openDB),
	}
	args := map[string][]string{"check run": {"--only", keyMisconfig}, "jobs show": {"1"}}

	isolateMisconfigEnv(t)
	for name, cmd := range commands {
		t.Run(name, func(t *testing.T) {
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append([]string{}, args[name]...))
			if err := cmd.Execute(); !errors.Is(err, openErr) {
				t.Fatalf("Execute() error = %v, want %v", err, openErr)
			}
		})
	}
}

func TestCheckRunValidatesChecksBeforeOpeningDatabase(t *testing.T) {
	openDB := func() (*Database, error) {
		t.Error("check run opened the database for an invalid --only value")
		return nil, errors.New("unexpected open")
	}

	cmd := newCheckRunCmd(openDB)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--only", "does-not-exist"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), `unknown check "does-not-exist"`) {
		t.Fatalf("Execute() error = %v, want unknown check", err)
	}
}

// neverOpen returns a DatabaseOpener that fails the test if it is called.
func neverOpen(t *testing.T) DatabaseOpener {
	t.Helper()
	return func() (*Database, error) {
		t.Error("check run opened the database")
		return nil, errors.New("unexpected open")
	}
}

func TestCheckRunPassesFlagValuesToChecks(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want CheckOptions
	}{
		{
			name: "defaults",
			want: CheckOptions{
				DiskPath:        "/",
				DiskWarnPercent: 80, DiskFailPercent: 90,
				MemWarnPercent: 80, MemFailPercent: 90,
				LoadWarnPercent: 80, LoadFailPercent: 100,
				CommandTimeout: 3 * time.Second,
			},
		},
		{
			name: "overrides",
			args: []string{
				"--disk-path", "/data", "--service", "nginx",
				"--disk-warn", "70", "--disk-fail", "85.5",
				"--mem-warn", "60", "--mem-fail", "95",
				"--load-warn", "150", "--load-fail", "300",
				"--timeout", "10s",
			},
			want: CheckOptions{
				DiskPath: "/data", ServiceName: "nginx",
				DiskWarnPercent: 70, DiskFailPercent: 85.5,
				MemWarnPercent: 60, MemFailPercent: 95,
				LoadWarnPercent: 150, LoadFailPercent: 300,
				CommandTimeout: 10 * time.Second,
			},
		},
		{
			name: "percentages at the 100 limit",
			args: []string{"--disk-fail", "100", "--mem-fail", "100"},
			want: CheckOptions{
				DiskPath:        "/",
				DiskWarnPercent: 80, DiskFailPercent: 100,
				MemWarnPercent: 80, MemFailPercent: 100,
				LoadWarnPercent: 80, LoadFailPercent: 100,
				CommandTimeout: 3 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got CheckOptions
			runChecks := func(keys []string, opts CheckOptions) ([]CheckOutcome, error) {
				got = opts
				return []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}}, nil
			}

			cmd := newCheckRunCmdWith(neverOpen(t), runChecks)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append([]string{"--no-save"}, tt.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			// The test seams are nil in both, and DeepEqual treats nil funcs as equal.
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("options passed to the checks = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCheckRunRejectsInvalidLimits(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"warn above default fail", []string{"--disk-warn", "95"}, "--disk-warn (95) must be less than --disk-fail (90)"},
		{"warn equals fail", []string{"--mem-warn", "90", "--mem-fail", "90"}, "--mem-warn (90) must be less than --mem-fail (90)"},
		{"load warn above fail", []string{"--load-warn", "200", "--load-fail", "150"}, "--load-warn (200) must be less than --load-fail (150)"},
		{"disk above 100", []string{"--disk-fail", "101"}, "invalid --disk-fail value 101: must be at most 100"},
		{"memory above 100", []string{"--mem-fail", "100.5"}, "invalid --mem-fail value 100.5: must be at most 100"},
		{"zero", []string{"--mem-warn", "0"}, "invalid --mem-warn value 0: must be a finite number greater than 0"},
		{"negative", []string{"--load-warn=-1"}, "invalid --load-warn value -1: must be a finite number greater than 0"},
		{"not a number", []string{"--load-fail", "NaN"}, "invalid --load-fail value NaN: must be a finite number greater than 0"},
		{"infinite", []string{"--load-fail", "+Inf"}, "invalid --load-fail value +Inf: must be a finite number greater than 0"},
		{"zero timeout", []string{"--timeout", "0s"}, "invalid --timeout value 0s: must be greater than 0"},
		{"negative timeout", []string{"--timeout=-1s"}, "invalid --timeout value -1s: must be greater than 0"},
		{"malformed value", []string{"--disk-warn", "high"}, `invalid argument "high" for "--disk-warn" flag`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runChecks := func([]string, CheckOptions) ([]CheckOutcome, error) {
				t.Error("checks ran despite an invalid flag value")
				return nil, nil
			}

			cmd := newCheckRunCmdWith(neverOpen(t), runChecks)
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Execute(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
			if got := ExitCode(err); got != ExitCodeError {
				t.Errorf("ExitCode(%v) = %d, want %d", err, got, ExitCodeError)
			}
		})
	}
}
