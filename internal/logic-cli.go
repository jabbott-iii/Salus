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
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// DatabaseOpener returns the database, opening it on first use. Commands call
// it only when they need storage, so help, --version, completion, and
// check run --no-save never create a database file.
type DatabaseOpener func() (*Database, error)

// NewRootCmd builds the Salus CLI command tree.
func NewRootCmd(openDB DatabaseOpener) *cobra.Command {
	root := &cobra.Command{
		Use:   "salus",
		Short: "Salus is an environment health checker",
		Long:  "Salus verifies disk space, memory, CPU load, Docker status, Kubernetes status, service uptime, and common misconfigurations.",
		// Cobra would print errors and usage through the output writer, which
		// mixes them into stdout (and --json). The caller reports errors on
		// stderr instead; see run in main.go.
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(newCheckCmd(openDB))
	root.AddCommand(newJobsCmd(openDB))

	return root
}

// runGroup shows help for a command group and rejects unknown subcommands.
// Without it Cobra prints help and exits 0 for a mistyped subcommand such as
// "salus check rn", which would let a broken CI step pass.
func runGroup(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "; did you mean " + strings.Join(suggestions, " or ") + "?"
	}
	return errors.New(msg)
}

func newCheckCmd(openDB DatabaseOpener) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run or list environment health checks",
		RunE:  runGroup,
		// Cobra only defaults this for the root command's suggestions.
		SuggestionsMinimumDistance: 2,
	}

	cmd.AddCommand(newCheckListCmd(openDB))
	cmd.AddCommand(newCheckRunCmd(openDB))

	return cmd
}

func newCheckListCmd(openDB DatabaseOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available health checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			features, err := ListFeatures(db)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			for _, f := range features {
				if _, err := fmt.Fprintf(out, "%-20s %-20s %s\n", f.Key, f.Category.Name, f.Description); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newCheckRunCmd(openDB DatabaseOpener) *cobra.Command {
	return newCheckRunCmdWith(openDB, RunChecks)
}

// newCheckRunCmdWith builds check run around the function that runs the
// checks, so tests can inspect the options produced by the flags.
func newCheckRunCmdWith(openDB DatabaseOpener, runChecks func([]string, CheckOptions) ([]CheckOutcome, error)) *cobra.Command {
	var (
		only       []string
		opts       CheckOptions
		jsonOutput bool
		failOnly   bool
		quiet      bool
		noSave     bool
	)

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run health checks and report the results",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ValidateCheckKeys(only); err != nil {
				return err
			}
			if err := validateLimits(opts); err != nil {
				return err
			}

			// Open storage before running checks, so a database problem is
			// reported before slow checks run rather than after.
			var db *Database
			if !noSave {
				var err error
				if db, err = openDB(); err != nil {
					return err
				}
			}

			startedAt := time.Now()
			outcomes, err := runChecks(only, opts)
			if err != nil {
				return err
			}

			if !noSave {
				if _, err := RecordScan(db, startedAt, outcomes); err != nil {
					return fmt.Errorf("record scan: %w", err)
				}
			}

			out := cmd.OutOrStdout()
			switch {
			case quiet:
				// --quiet suppresses report output, including --json.
			case jsonOutput:
				if err := WriteOutcomesJSON(out, outcomes); err != nil {
					return err
				}
			default:
				if err := WriteOutcomesText(out, "Environment Health Check", outcomes, failOnly); err != nil {
					return err
				}
			}

			if code := ExitCodeFor(WorstStatus(outcomes)); code != ExitCodePass {
				// A WARN or FAIL result is not a usage problem, so Cobra must not
				// print the error or the usage text; main maps it to the exit code.
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				return &ExitStatusError{Code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&only, "only", nil, "comma-separated list of checks to run (default: all)")
	cmd.Flags().StringVar(&opts.ServiceName, "service", "", "systemd service name to check uptime for (defaults to host uptime)")
	cmd.Flags().StringVar(&opts.KubeContext, "kube-context", "", "kubeconfig context for the kubernetes-status check (defaults to kubectl's current context)")
	cmd.Flags().StringVar(&opts.DiskPath, "disk-path", "/", "mount path to check for free disk space")
	cmd.Flags().Float64Var(&opts.DiskWarnPercent, "disk-warn", defaultDiskWarnPercent, "disk usage percent at which disk-space reports WARN")
	cmd.Flags().Float64Var(&opts.DiskFailPercent, "disk-fail", defaultDiskFailPercent, "disk usage percent at which disk-space reports FAIL")
	cmd.Flags().Float64Var(&opts.MemWarnPercent, "mem-warn", defaultMemWarnPercent, "memory usage percent at which memory reports WARN")
	cmd.Flags().Float64Var(&opts.MemFailPercent, "mem-fail", defaultMemFailPercent, "memory usage percent at which memory reports FAIL")
	cmd.Flags().Float64Var(&opts.LoadWarnPercent, "load-warn", defaultLoadWarnPercent, "1-minute load average per CPU, in percent, at which cpu-load reports WARN")
	cmd.Flags().Float64Var(&opts.LoadFailPercent, "load-fail", defaultLoadFailPercent, "1-minute load average per CPU, in percent, at which cpu-load reports FAIL")
	cmd.Flags().DurationVar(&opts.CommandTimeout, "timeout", defaultCommandTimeout, "time limit for each docker, kubectl, or systemctl command")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output results as JSON")
	cmd.Flags().BoolVar(&failOnly, "fail-only", false, "only show WARN and FAIL results in text output")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress report output, including --json (still sets the exit code)")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "do not persist this run to the database")

	return cmd
}

// validateLimits rejects threshold and timeout flag values that cannot work:
// every threshold must be a finite number above 0, each warn threshold must be
// below its fail threshold, disk and memory thresholds are percentages of
// capacity (at most 100), and the timeout must be positive. Load can exceed
// 100 percent.
func validateLimits(opts CheckOptions) error {
	pairs := []struct {
		name       string
		warn, fail float64
		max        float64 // 0 means no upper bound
	}{
		{"disk", opts.DiskWarnPercent, opts.DiskFailPercent, 100},
		{"mem", opts.MemWarnPercent, opts.MemFailPercent, 100},
		{"load", opts.LoadWarnPercent, opts.LoadFailPercent, 0},
	}

	for _, p := range pairs {
		for _, f := range []struct {
			flag  string
			value float64
		}{{"--" + p.name + "-warn", p.warn}, {"--" + p.name + "-fail", p.fail}} {
			// pflag parses "NaN" and "Inf", so they are rejected explicitly.
			if math.IsNaN(f.value) || math.IsInf(f.value, 0) || f.value <= 0 {
				return fmt.Errorf("invalid %s value %g: must be a finite number greater than 0", f.flag, f.value)
			}
			if p.max > 0 && f.value > p.max {
				return fmt.Errorf("invalid %s value %g: must be at most %g", f.flag, f.value, p.max)
			}
		}
		if p.warn >= p.fail {
			return fmt.Errorf("--%s-warn (%g) must be less than --%s-fail (%g)", p.name, p.warn, p.name, p.fail)
		}
	}

	if opts.CommandTimeout <= 0 {
		return fmt.Errorf("invalid --timeout value %s: must be greater than 0", opts.CommandTimeout)
	}
	return nil
}

func newJobsCmd(openDB DatabaseOpener) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "View past health check runs",
		RunE:  runGroup,
		// Cobra only defaults this for the root command's suggestions.
		SuggestionsMinimumDistance: 2,
	}

	cmd.AddCommand(newJobsListCmd(openDB))
	cmd.AddCommand(newJobsShowCmd(openDB))
	cmd.AddCommand(newJobsPruneCmd(openDB))

	return cmd
}

func newJobsListCmd(openDB DatabaseOpener) *cobra.Command {
	var (
		limit      int
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent health check runs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := openDB()
			if err != nil {
				return err
			}
			jobs, err := ListScanJobs(db, limit)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOutput {
				return WriteJobsJSON(out, jobs)
			}
			for _, j := range jobs {
				finished := "running"
				if j.FinishedAt != nil {
					finished = j.FinishedAt.Format(time.RFC3339)
				}
				if _, err := fmt.Fprintf(out, "%-4d %-9s %-25s %-25s %s\n", j.ID, j.Status, j.StartedAt.Format(time.RFC3339), finished, j.Summary); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "maximum number of jobs to list")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output jobs as JSON")

	return cmd
}

func newJobsShowCmd(openDB DatabaseOpener) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "show [job-id]",
		Short: "Show details for a specific health check run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseUint(args[0], 10, strconv.IntSize)
			if err != nil {
				return fmt.Errorf("invalid job id %q: %w", args[0], err)
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			job, results, err := GetScanJob(db, uint(id))
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if jsonOutput {
				return WriteJobJSON(out, job, results)
			}
			if _, err := fmt.Fprintf(out, "Job %d: %s (%s)\n", job.ID, job.Status, job.Summary); err != nil {
				return err
			}
			for _, r := range results {
				// Rows written before SEC-009 may still hold control characters.
				if _, err := fmt.Fprintf(out, "[%s] %-17s %s\n", r.Status, r.Key, sanitizeMessage(r.Message)); err != nil {
					return err
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the job and its results as JSON")

	return cmd
}

func newJobsPruneCmd(openDB DatabaseOpener) *cobra.Command {
	var (
		olderThan string
		dryRun    bool
	)

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Delete health check runs older than a given age",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if olderThan == "" {
				return errors.New("--older-than is required, for example --older-than 30d")
			}
			age, err := parseAge(olderThan)
			if err != nil {
				return fmt.Errorf("invalid --older-than value %q: %w", olderThan, err)
			}

			db, err := openDB()
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-age)
			n, err := PruneScanJobs(db, cutoff, dryRun)
			if err != nil {
				return err
			}

			verb, noun := "Deleted", "jobs"
			if dryRun {
				verb = "Would delete"
			}
			if n == 1 {
				noun = "job"
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %d %s started before %s\n", verb, n, noun, cutoff.Format(time.RFC3339))
			return err
		},
	}

	cmd.Flags().StringVar(&olderThan, "older-than", "", "delete runs that started longer ago than this age, such as 30d, 12h, or 90m (required)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report how many runs would be deleted without deleting them")

	return cmd
}

// maxAgeDays keeps a day count within time.Duration's range (about 292 years).
const maxAgeDays = math.MaxInt64 / int64(24*time.Hour)

var errAgeFormat = errors.New("use a whole number of days such as 30d, or a duration such as 12h")

// parseAge parses a positive age: a whole number of days such as 30d, or a Go
// duration such as 12h or 90m.
func parseAge(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.ParseInt(days, 10, 64)
		switch {
		case err != nil:
			return 0, errAgeFormat
		case n <= 0:
			return 0, errors.New("must be greater than 0")
		case n > maxAgeDays:
			return 0, fmt.Errorf("must be at most %dd", maxAgeDays)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}

	age, err := time.ParseDuration(s)
	if err != nil {
		return 0, errAgeFormat
	}
	if age <= 0 {
		return 0, errors.New("must be greater than 0")
	}
	return age, nil
}
