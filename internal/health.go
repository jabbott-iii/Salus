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
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Built-in health check keys.
const (
	keyDiskSpace     = "disk-space"
	keyMemory        = "memory"
	keyCPULoad       = "cpu-load"
	keyDocker        = "docker-status"
	keyKubernetes    = "kubernetes-status"
	keyServiceUptime = "service-uptime"
	keyMisconfig     = "misconfig"
)

// AllCheckKeys lists every built-in check key in default run order.
var AllCheckKeys = []string{
	keyDiskSpace,
	keyMemory,
	keyCPULoad,
	keyDocker,
	keyKubernetes,
	keyServiceUptime,
	keyMisconfig,
}

// CheckStatus represents the severity of a single health check outcome.
type CheckStatus string

// Supported check statuses, ordered from healthiest to least healthy.
const (
	StatusPass CheckStatus = "PASS"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckOutcome captures the result of running a single health check.
type CheckOutcome struct {
	Key      string        `json:"key"`
	Status   CheckStatus   `json:"status"`
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration_ns"`
}

// CheckOptions configures thresholds and targets used by the built-in health checks.
type CheckOptions struct {
	DiskPath        string
	DiskWarnPercent float64
	DiskFailPercent float64

	MemWarnPercent float64
	MemFailPercent float64

	LoadWarnPercent float64
	LoadFailPercent float64

	ServiceName string

	CommandTimeout time.Duration

	// Test seams for external tools; nil means exec.CommandContext(...).CombinedOutput
	// and exec.LookPath. Tests set them so no real docker/kubectl/systemctl runs.
	runCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookPath   func(file string) (string, error)
}

// defaultCommandTimeout applies when CheckOptions.CommandTimeout is unset (<= 0).
// Resource threshold defaults live in health-thresholds.go.
const defaultCommandTimeout = 3 * time.Second

func (o CheckOptions) commandTimeout() time.Duration {
	if o.CommandTimeout <= 0 {
		return defaultCommandTimeout
	}
	return o.CommandTimeout
}

// command runs an external tool with a timeout and returns its combined output.
func (o CheckOptions) command(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), o.commandTimeout())
	defer cancel()

	if o.runCommand != nil {
		return o.runCommand(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

// hasTool reports whether an executable is available on PATH.
func (o CheckOptions) hasTool(file string) bool {
	lookPath := exec.LookPath
	if o.lookPath != nil {
		lookPath = o.lookPath
	}
	_, err := lookPath(file)
	return err == nil
}

// thresholdStatus classifies value against warn/fail thresholds, attaching detail as the message.
func thresholdStatus(value, warn, fail float64, detail string) (CheckStatus, string) {
	switch {
	case value >= fail:
		return StatusFail, detail
	case value >= warn:
		return StatusWarn, detail
	default:
		return StatusPass, detail
	}
}

// firstLine extracts a short, human-readable message from command output or an error.
func firstLine(output string, err error) string {
	output = strings.TrimSpace(output)
	if output != "" {
		return strings.SplitN(output, "\n", 2)[0]
	}
	if err != nil {
		return err.Error()
	}
	return "unknown error"
}

type checkFunc func(CheckOptions) CheckOutcome

var checkRegistry = map[string]checkFunc{
	keyDiskSpace:     checkDiskSpace,
	keyMemory:        checkMemory,
	keyCPULoad:       checkCPULoad,
	keyDocker:        checkDockerStatus,
	keyKubernetes:    checkKubernetesStatus,
	keyServiceUptime: checkServiceUptime,
	keyMisconfig:     checkMisconfiguration,
}

// RunChecks executes the given check keys (or all built-in checks when keys is empty)
// and returns their outcomes in the order requested.
func RunChecks(keys []string, opts CheckOptions) ([]CheckOutcome, error) {
	if len(keys) == 0 {
		keys = AllCheckKeys
	}

	outcomes := make([]CheckOutcome, 0, len(keys))
	for _, key := range keys {
		fn, ok := checkRegistry[key]
		if !ok {
			return nil, fmt.Errorf("unknown check %q", key)
		}
		outcomes = append(outcomes, fn(opts))
	}
	return outcomes, nil
}

//--------------------------------------------------container & orchestration checks-------------------------------------------------------------------//

func checkDockerStatus(opts CheckOptions) CheckOutcome {
	start := time.Now()

	if !opts.hasTool("docker") {
		return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: "docker CLI not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command("docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		return CheckOutcome{Key: keyDocker, Status: StatusFail, Message: fmt.Sprintf("docker daemon unreachable: %s", firstLine(string(out), err)), Duration: time.Since(start)}
	}

	version := strings.TrimSpace(string(out))
	return CheckOutcome{Key: keyDocker, Status: StatusPass, Message: fmt.Sprintf("docker daemon reachable (server version %s)", version), Duration: time.Since(start)}
}

func checkKubernetesStatus(opts CheckOptions) CheckOutcome {
	start := time.Now()

	if !opts.hasTool("kubectl") {
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: "kubectl CLI not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command("kubectl", "cluster-info")
	if err != nil {
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("kubernetes cluster unreachable: %s", firstLine(string(out), err)), Duration: time.Since(start)}
	}

	return CheckOutcome{Key: keyKubernetes, Status: StatusPass, Message: "kubernetes cluster reachable", Duration: time.Since(start)}
}

//--------------------------------------------------service & configuration checks---------------------------------------------------------------------//

func checkServiceUptime(opts CheckOptions) CheckOutcome {
	start := time.Now()
	name := strings.TrimSpace(opts.ServiceName)

	if name == "" {
		return checkProcessUptime(start)
	}

	if runtime.GOOS != "linux" {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "service uptime check requires systemd (Linux only)", Duration: time.Since(start)}
	}

	if !opts.hasTool("systemctl") {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "systemctl not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command("systemctl", "is-active", name)
	// Only the first line is used so messages stay single-line in reports.
	state := firstLine(string(out), err)

	switch {
	case err == nil && state == "active":
		return CheckOutcome{Key: keyServiceUptime, Status: StatusPass, Message: fmt.Sprintf("service %q is active", name), Duration: time.Since(start)}
	case state == "activating" || state == "reloading":
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: fmt.Sprintf("service %q is %s", name, state), Duration: time.Since(start)}
	default:
		return CheckOutcome{Key: keyServiceUptime, Status: StatusFail, Message: fmt.Sprintf("service %q is not active (%s)", name, state), Duration: time.Since(start)}
	}
}

func checkProcessUptime(start time.Time) CheckOutcome {
	uptime, err := readSystemUptime()
	if err != nil {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "no --service provided and host uptime unavailable: " + err.Error(), Duration: time.Since(start)}
	}
	return CheckOutcome{Key: keyServiceUptime, Status: StatusPass, Message: fmt.Sprintf("host has been up for %s", uptime.Round(time.Second)), Duration: time.Since(start)}
}

func checkMisconfiguration(opts CheckOptions) CheckOutcome {
	start := time.Now()
	var warnings []string

	if _, ok := os.LookupEnv("HOME"); !ok && runtime.GOOS != "windows" {
		warnings = append(warnings, "HOME environment variable is not set")
	}

	// On Windows, Go reports 0666 for every writable file (ACLs are not mode
	// bits), so this POSIX permission test would always warn there.
	if dbPath := os.Getenv(DatabasePathEnv); dbPath != "" && runtime.GOOS != "windows" {
		if info, err := os.Stat(dbPath); err == nil {
			if info.Mode().Perm()&0o022 != 0 {
				warnings = append(warnings, fmt.Sprintf("%s is writable by group/other", dbPath))
			}
		}
	}

	if len(warnings) == 0 {
		return CheckOutcome{Key: keyMisconfig, Status: StatusPass, Message: "no common misconfigurations detected", Duration: time.Since(start)}
	}

	return CheckOutcome{Key: keyMisconfig, Status: StatusWarn, Message: strings.Join(warnings, "; "), Duration: time.Since(start)}
}
