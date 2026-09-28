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
	var (
		only       []string
		service    string
		diskPath   string
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
			opts := CheckOptions{
				DiskPath:    diskPath,
				ServiceName: service,
			}

			if err := ValidateCheckKeys(only); err != nil {
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
			outcomes, err := RunChecks(only, opts)
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
	cmd.Flags().StringVar(&service, "service", "", "systemd service name to check uptime for (defaults to host uptime)")
	cmd.Flags().StringVar(&diskPath, "disk-path", "/", "mount path to check for free disk space")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output results as JSON")
	cmd.Flags().BoolVar(&failOnly, "fail-only", false, "only show WARN and FAIL results in text output")
	cmd.Flags().BoolVar(&quiet, "quiet", false, "suppress report output, including --json (still sets the exit code)")
	cmd.Flags().BoolVar(&noSave, "no-save", false, "do not persist this run to the database")

	return cmd
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

	return cmd
}

func newJobsListCmd(openDB DatabaseOpener) *cobra.Command {
	var limit int

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

	return cmd
}

func newJobsShowCmd(openDB DatabaseOpener) *cobra.Command {
	return &cobra.Command{
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
			if _, err := fmt.Fprintf(out, "Job %d: %s (%s)\n", job.ID, job.Status, job.Summary); err != nil {
				return err
			}
			for _, r := range results {
				if _, err := fmt.Fprintf(out, "[%s] %-17s %s\n", r.Status, r.Key, r.Message); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
