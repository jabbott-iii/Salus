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
