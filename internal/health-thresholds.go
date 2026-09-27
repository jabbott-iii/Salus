//go:build linux

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

// Threshold defaults and accessors for the disk, memory, and CPU load checks.
// Only the Linux resource checks use them today, so this file is built only on
// Linux; otherwise the unused linter fails on the non-Linux stubs. Widen the
// build constraint when the macOS and Windows checks land (intel/plan.md P3-7).

// Default thresholds applied when the corresponding CheckOptions field is unset (<= 0).
const (
	defaultDiskWarnPercent = 80.0
	defaultDiskFailPercent = 90.0
	defaultMemWarnPercent  = 80.0
	defaultMemFailPercent  = 90.0
	defaultLoadWarnPercent = 80.0
	defaultLoadFailPercent = 100.0
)

func orDefault(v, def float64) float64 {
	if v <= 0 {
		return def
	}
	return v
}

func (o CheckOptions) diskWarnPercent() float64 {
	return orDefault(o.DiskWarnPercent, defaultDiskWarnPercent)
}
func (o CheckOptions) diskFailPercent() float64 {
	return orDefault(o.DiskFailPercent, defaultDiskFailPercent)
}
func (o CheckOptions) memWarnPercent() float64 {
	return orDefault(o.MemWarnPercent, defaultMemWarnPercent)
}
func (o CheckOptions) memFailPercent() float64 {
	return orDefault(o.MemFailPercent, defaultMemFailPercent)
}
func (o CheckOptions) loadWarnPercent() float64 {
	return orDefault(o.LoadWarnPercent, defaultLoadWarnPercent)
}
func (o CheckOptions) loadFailPercent() float64 {
	return orDefault(o.LoadFailPercent, defaultLoadFailPercent)
}
