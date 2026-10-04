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
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const meminfoFixture = `MemTotal:        8000000 kB
MemFree:          500000 kB
MemAvailable:    2000000 kB
Buffers:          100000 kB
SwapTotal:       1000000 kB
SwapFree:         750000 kB
Malformed line
HugePages_Total:       0
`

func TestParseMeminfo(t *testing.T) {
	got := parseMeminfo([]byte(meminfoFixture))

	want := meminfo{totalKB: 8000000, availableKB: 2000000, swapTotalKB: 1000000, swapFreeKB: 750000}
	if got != want {
		t.Fatalf("parseMeminfo() = %+v, want %+v", got, want)
	}
	if swap := got.swapPercent(); swap != 25 {
		t.Errorf("swapPercent() = %v, want 25", swap)
	}
	if empty := parseMeminfo(nil); empty.swapPercent() != 0 {
		t.Errorf("swapPercent() without swap = %v, want 0", empty.swapPercent())
	}
}

func TestParseLoadAverage(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    float64
		wantErr bool
	}{
		{name: "typical", data: "0.52 0.58 0.59 1/467 12345\n", want: 0.52},
		{name: "empty", data: "", wantErr: true},
		{name: "not a number", data: "abc 0.58 0.59", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLoadAverage([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseLoadAverage() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseLoadAverage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseUptime(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    time.Duration
		wantErr bool
	}{
		{name: "typical", data: "3725.50 7000.10\n", want: 3725500 * time.Millisecond},
		{name: "empty", data: "\n", wantErr: true},
		{name: "not a number", data: "up 7000.10", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUptime([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseUptime() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseUptime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThresholdAccessorsUseDefaultsAndOverrides(t *testing.T) {
	defaults := CheckOptions{}
	overrides := CheckOptions{DiskWarnPercent: 50, DiskFailPercent: 60, InodeWarnPercent: 40, InodeFailPercent: 45, MemWarnPercent: 55, MemFailPercent: 65, LoadWarnPercent: 70, LoadFailPercent: 150}

	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"disk warn default", defaults.diskWarnPercent(), defaultDiskWarnPercent},
		{"disk fail default", defaults.diskFailPercent(), defaultDiskFailPercent},
		{"mem warn default", defaults.memWarnPercent(), defaultMemWarnPercent},
		{"mem fail default", defaults.memFailPercent(), defaultMemFailPercent},
		{"load warn default", defaults.loadWarnPercent(), defaultLoadWarnPercent},
		{"load fail default", defaults.loadFailPercent(), defaultLoadFailPercent},
		{"disk warn override", overrides.diskWarnPercent(), 50},
		{"disk fail override", overrides.diskFailPercent(), 60},
		{"mem warn override", overrides.memWarnPercent(), 55},
		{"mem fail override", overrides.memFailPercent(), 65},
		{"load warn override", overrides.loadWarnPercent(), 70},
		{"load fail override", overrides.loadFailPercent(), 150},
		{"inode warn default", defaults.inodeWarnPercent(), defaultInodeWarnPercent},
		{"inode fail default", defaults.inodeFailPercent(), defaultInodeFailPercent},
		{"inode warn override", overrides.inodeWarnPercent(), 40},
		{"inode fail override", overrides.inodeFailPercent(), 45},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestCheckDiskSpaceMissingPathFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	got := checkDiskSpace(CheckOptions{DiskPath: missing})
	if got.Status != StatusFail || !strings.HasPrefix(got.Message, "statfs "+missing) {
		t.Fatalf("checkDiskSpace() = {%s %q}, want FAIL starting with %q", got.Status, got.Message, "statfs "+missing)
	}
}

func TestInodeOutcome(t *testing.T) {
	tests := []struct {
		name        string
		files       uint64
		ffree       uint64
		opts        CheckOptions
		wantStatus  CheckStatus
		wantMessage string
		wantValue   float64
	}{
		{"plenty free", 1000, 900, CheckOptions{}, StatusPass, "/: 10.0% of inodes used (900 free)", 10},
		{"warn at default threshold", 1000, 200, CheckOptions{}, StatusWarn, "/: 80.0% of inodes used (200 free)", 80},
		{"fail at default threshold", 1000, 100, CheckOptions{}, StatusFail, "/: 90.0% of inodes used (100 free)", 90},
		{"custom thresholds", 1000, 600, CheckOptions{InodeWarnPercent: 30, InodeFailPercent: 50}, StatusWarn, "/: 40.0% of inodes used (600 free)", 40},
		{"none free", 1000, 0, CheckOptions{}, StatusFail, "/: 100.0% of inodes used (0 free)", 100},
		{"free above total is clamped", 10, 20, CheckOptions{}, StatusPass, "/: 0.0% of inodes used (10 free)", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := inodeOutcome("/", tt.files, tt.ffree, tt.opts)
			assertOutcome(t, got, keyDiskInodes, tt.wantStatus, tt.wantMessage)
			if got.Value == nil || *got.Value != tt.wantValue || got.Unit != unitPercent {
				t.Errorf("value = %v %s, want %v percent", got.Value, got.Unit, tt.wantValue)
			}
		})
	}

	got := inodeOutcome("/data", 0, 0, CheckOptions{})
	assertOutcome(t, got, keyDiskInodes, StatusPass, "/data: filesystem does not report an inode count")
	if got.Value != nil {
		t.Errorf("value = %v, want none when the filesystem reports no inodes", *got.Value)
	}
}

func TestCheckDiskInodes(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	got := checkDiskInodes(CheckOptions{DiskPath: missing})
	if got.Status != StatusFail || !strings.HasPrefix(got.Message, "statfs "+missing) {
		t.Errorf("checkDiskInodes(missing) = {%s %q}, want FAIL starting with %q", got.Status, got.Message, "statfs "+missing)
	}
}

// TestResourceChecksReportValues reads the host's /proc and statfs, but
// asserts only that a value is attached, which holds for any resource level.
func TestResourceChecksReportValues(t *testing.T) {
	for _, check := range []func(CheckOptions) CheckOutcome{checkDiskSpace, checkMemory, checkCPULoad} {
		got := check(CheckOptions{})
		if got.Status != StatusFail && (got.Value == nil || got.Unit != unitPercent) {
			t.Errorf("%s = %+v, want a percent value", got.Key, got)
		}
	}
}
