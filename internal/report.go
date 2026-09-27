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
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// WriteOutcomesText renders check outcomes as a human-readable, aligned report.
// When failOnly is true, only WARN and FAIL outcomes are printed.
func WriteOutcomesText(w io.Writer, title string, outcomes []CheckOutcome, failOnly bool) error {
	if title != "" {
		if _, err := fmt.Fprintln(w, title); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	for _, o := range outcomes {
		if failOnly && o.Status == StatusPass {
			continue
		}
		if _, err := fmt.Fprintf(w, "[%s] %-17s %s\n", o.Status, o.Key, o.Message); err != nil {
			return err
		}
	}
	return nil
}

// WriteOutcomesJSON renders check outcomes as JSON.
func WriteOutcomesJSON(w io.Writer, outcomes []CheckOutcome) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(outcomes)
}

// WorstStatus returns the most severe status among the given outcomes,
// or StatusPass if outcomes is empty.
func WorstStatus(outcomes []CheckOutcome) CheckStatus {
	worst := StatusPass
	for _, o := range outcomes {
		switch o.Status {
		case StatusFail:
			return StatusFail
		case StatusWarn:
			worst = StatusWarn
		}
	}
	return worst
}

// Process exit codes. ExitCodeError covers anything that prevents a complete
// run: invalid flags or arguments, unknown check keys, and database failures.
const (
	ExitCodePass  = 0
	ExitCodeWarn  = 1
	ExitCodeFail  = 2
	ExitCodeError = 3
)

// ExitCodeFor maps a worst-case status to a process exit code:
// 0 for PASS, 1 for WARN, and 2 for FAIL.
func ExitCodeFor(status CheckStatus) int {
	switch status {
	case StatusFail:
		return ExitCodeFail
	case StatusWarn:
		return ExitCodeWarn
	default:
		return ExitCodePass
	}
}

// ExitStatusError reports a completed check run whose worst status maps to a
// non-zero exit code. It is returned from a command so that the exit code
// reaches main without os.Exit inside the command; it is not an operational
// error.
type ExitStatusError struct {
	Code int
}

func (e *ExitStatusError) Error() string {
	return fmt.Sprintf("health checks finished with exit status %d", e.Code)
}

// ExitCode maps the error returned by the root command to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitCodePass
	}
	var status *ExitStatusError
	if errors.As(err, &status) {
		return status.Code
	}
	return ExitCodeError
}
