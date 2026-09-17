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
	"fmt"
	"io"
)

// WriteOutcomesText renders check outcomes as a human-readable, aligned report.
// When failOnly is true, only WARN and FAIL outcomes are printed.
func WriteOutcomesText(w io.Writer, title string, outcomes []CheckOutcome, failOnly bool) {
	if title != "" {
		if _, err := fmt.Fprintln(w, title); err != nil {
			return
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return
		}
	}

	for _, o := range outcomes {
		if failOnly && o.Status == StatusPass {
			continue
		}
		if _, err := fmt.Fprintf(w, "[%s] %-17s %s\n", o.Status, o.Key, o.Message); err != nil {
			return
		}
	}
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

// ExitCodeFor maps a worst-case status to a process exit code:
// 0 for PASS, 1 for WARN, and 2 for FAIL.
func ExitCodeFor(status CheckStatus) int {
	switch status {
	case StatusFail:
		return 2
	case StatusWarn:
		return 1
	default:
		return 0
	}
}
