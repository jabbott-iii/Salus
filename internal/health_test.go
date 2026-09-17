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
	"strings"
	"testing"
)

func TestThresholdStatus(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		want  CheckStatus
	}{
		{name: "below warn", value: 10, want: StatusPass},
		{name: "at warn", value: 80, want: StatusWarn},
		{name: "between warn and fail", value: 85, want: StatusWarn},
		{name: "at fail", value: 90, want: StatusFail},
		{name: "above fail", value: 99, want: StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _ := thresholdStatus(tt.value, 80, 90, "detail")
			if status != tt.want {
				t.Errorf("thresholdStatus(%v) = %v, want %v", tt.value, status, tt.want)
			}
		})
	}
}

func TestRunChecksDefaultsToAllChecks(t *testing.T) {
	outcomes, err := RunChecks(nil, CheckOptions{})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if len(outcomes) != len(AllCheckKeys) {
		t.Fatalf("RunChecks() returned %d outcomes, want %d", len(outcomes), len(AllCheckKeys))
	}
	for i, outcome := range outcomes {
		if outcome.Key != AllCheckKeys[i] {
			t.Errorf("outcomes[%d].Key = %q, want %q", i, outcome.Key, AllCheckKeys[i])
		}
	}
}

func TestRunChecksSubset(t *testing.T) {
	outcomes, err := RunChecks([]string{keyMisconfig}, CheckOptions{})
	if err != nil {
		t.Fatalf("RunChecks() error = %v", err)
	}
	if len(outcomes) != 1 || outcomes[0].Key != keyMisconfig {
		t.Fatalf("RunChecks() = %+v, want single misconfig outcome", outcomes)
	}
}

func TestRunChecksUnknownKey(t *testing.T) {
	if _, err := RunChecks([]string{"does-not-exist"}, CheckOptions{}); err == nil {
		t.Fatal("RunChecks() expected error for unknown check key, got nil")
	}
}

func TestWorstStatus(t *testing.T) {
	tests := []struct {
		name     string
		outcomes []CheckOutcome
		want     CheckStatus
	}{
		{name: "empty", outcomes: nil, want: StatusPass},
		{name: "all pass", outcomes: []CheckOutcome{{Status: StatusPass}, {Status: StatusPass}}, want: StatusPass},
		{name: "warn present", outcomes: []CheckOutcome{{Status: StatusPass}, {Status: StatusWarn}}, want: StatusWarn},
		{name: "fail wins", outcomes: []CheckOutcome{{Status: StatusWarn}, {Status: StatusFail}, {Status: StatusPass}}, want: StatusFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorstStatus(tt.outcomes); got != tt.want {
				t.Errorf("WorstStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		status CheckStatus
		want   int
	}{
		{status: StatusPass, want: 0},
		{status: StatusWarn, want: 1},
		{status: StatusFail, want: 2},
	}

	for _, tt := range tests {
		if got := ExitCodeFor(tt.status); got != tt.want {
			t.Errorf("ExitCodeFor(%v) = %d, want %d", tt.status, got, tt.want)
		}
	}
}

func TestWriteOutcomesTextFailOnly(t *testing.T) {
	outcomes := []CheckOutcome{
		{Key: "a", Status: StatusPass, Message: "ok"},
		{Key: "b", Status: StatusWarn, Message: "careful"},
		{Key: "c", Status: StatusFail, Message: "broken"},
	}

	var buf bytes.Buffer
	if err := WriteOutcomesText(&buf, "", outcomes, true); err != nil {
		t.Fatalf("WriteOutcomesText() error = %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "] a ") {
		t.Errorf("fail-only output unexpectedly contains passing check: %q", out)
	}
	if !strings.Contains(out, "[WARN] b") || !strings.Contains(out, "[FAIL] c") {
		t.Errorf("fail-only output missing WARN/FAIL entries: %q", out)
	}
}

func TestWriteOutcomesJSON(t *testing.T) {
	outcomes := []CheckOutcome{{Key: "a", Status: StatusPass, Message: "ok"}}

	var buf bytes.Buffer
	if err := WriteOutcomesJSON(&buf, outcomes); err != nil {
		t.Fatalf("WriteOutcomesJSON() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"key": "a"`) {
		t.Errorf("WriteOutcomesJSON() output = %q, missing expected key field", buf.String())
	}
}
