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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeResult is the canned combined output and error for one simulated command.
type fakeResult struct {
	out string
	err error
}

// fakeToolOptions returns CheckOptions whose external tools are simulated.
// installed lists the tools present on PATH; results maps "name arg..." to the
// command's result. It also returns the commands that were run, in order.
func fakeToolOptions(t *testing.T, installed []string, results map[string]fakeResult) (CheckOptions, *[]string) {
	t.Helper()

	var calls []string
	opts := CheckOptions{
		lookPath: func(file string) (string, error) {
			for _, name := range installed {
				if name == file {
					return filepath.Join("fakebin", file), nil
				}
			}
			return "", exec.ErrNotFound
		},
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			command := strings.Join(append([]string{name}, args...), " ")
			calls = append(calls, command)
			result, ok := results[command]
			if !ok {
				t.Errorf("unexpected command %q", command)
				return nil, errors.New("unexpected command")
			}
			return []byte(result.out), result.err
		},
	}
	return opts, &calls
}

// unsetEnv removes key for the duration of the test and restores it afterwards.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	previous, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, previous)
		}
	})
}

// isolateMisconfigEnv gives the misconfig check a clean, passing environment:
// no SALUS_DB_PATH, and a per-user default database location in a temp dir
// that does not exist yet.
func isolateMisconfigEnv(t *testing.T) {
	t.Helper()

	t.Setenv(DatabasePathEnv, "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", t.TempDir())
	}
}

func assertOutcome(t *testing.T, got CheckOutcome, wantKey string, wantStatus CheckStatus, wantMessage string) {
	t.Helper()

	if got.Key != wantKey || got.Status != wantStatus || got.Message != wantMessage {
		t.Errorf("outcome = {%s %s %q}, want {%s %s %q}", got.Key, got.Status, got.Message, wantKey, wantStatus, wantMessage)
	}
	if strings.Contains(got.Message, "\n") {
		t.Errorf("outcome message %q spans multiple lines", got.Message)
	}
}

func TestCheckDockerStatus(t *testing.T) {
	dockerInfo := "docker info --format {{.ServerVersion}}"
	tests := []struct {
		name        string
		installed   []string
		results     map[string]fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{
			name:        "CLI missing",
			wantStatus:  StatusWarn,
			wantMessage: "docker CLI not found in PATH",
		},
		{
			name:        "daemon unreachable",
			installed:   []string{"docker"},
			results:     map[string]fakeResult{dockerInfo: {out: "Cannot connect to the Docker daemon\nIs the docker daemon running?\n", err: errors.New("exit status 1")}},
			wantStatus:  StatusFail,
			wantMessage: "docker daemon unreachable: Cannot connect to the Docker daemon",
		},
		{
			name:        "daemon reachable",
			installed:   []string{"docker"},
			results:     map[string]fakeResult{dockerInfo: {out: "27.3.1\n"}},
			wantStatus:  StatusPass,
			wantMessage: "docker daemon reachable (server version 27.3.1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, calls := fakeToolOptions(t, tt.installed, tt.results)
			assertOutcome(t, checkDockerStatus(opts), keyDocker, tt.wantStatus, tt.wantMessage)
			if len(tt.installed) == 0 && len(*calls) != 0 {
				t.Errorf("commands run without docker installed: %q", *calls)
			}
		})
	}
}

func TestCheckKubernetesStatus(t *testing.T) {
	clusterInfo := "kubectl cluster-info"
	tests := []struct {
		name        string
		installed   []string
		results     map[string]fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{
			name:        "CLI missing",
			wantStatus:  StatusWarn,
			wantMessage: "kubectl CLI not found in PATH",
		},
		{
			name:        "cluster unreachable with no output",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: {err: errors.New("exit status 1")}},
			wantStatus:  StatusFail,
			wantMessage: "kubernetes cluster unreachable: exit status 1",
		},
		{
			name:        "cluster reachable",
			installed:   []string{"kubectl"},
			results:     map[string]fakeResult{clusterInfo: {out: "Kubernetes control plane is running\n"}},
			wantStatus:  StatusPass,
			wantMessage: "kubernetes cluster reachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, tt.results)
			assertOutcome(t, checkKubernetesStatus(opts), keyKubernetes, tt.wantStatus, tt.wantMessage)
		})
	}
}

func TestCheckServiceUptimeWithService(t *testing.T) {
	isActive := "systemctl is-active -- nginx"

	if runtime.GOOS != "linux" {
		opts, calls := fakeToolOptions(t, []string{"systemctl"}, nil)
		opts.ServiceName = "nginx"
		assertOutcome(t, checkServiceUptime(opts), keyServiceUptime, StatusWarn, "service uptime check requires systemd (Linux only)")
		if len(*calls) != 0 {
			t.Errorf("commands run on %s: %q", runtime.GOOS, *calls)
		}
		return
	}

	tests := []struct {
		name        string
		installed   []string
		results     map[string]fakeResult
		wantStatus  CheckStatus
		wantMessage string
	}{
		{
			name:        "systemctl missing",
			wantStatus:  StatusWarn,
			wantMessage: "systemctl not found in PATH",
		},
		{
			name:        "active",
			installed:   []string{"systemctl"},
			results:     map[string]fakeResult{isActive: {out: "active\n"}},
			wantStatus:  StatusPass,
			wantMessage: `service "nginx" is active`,
		},
		{
			name:        "activating",
			installed:   []string{"systemctl"},
			results:     map[string]fakeResult{isActive: {out: "activating\n", err: errors.New("exit status 3")}},
			wantStatus:  StatusWarn,
			wantMessage: `service "nginx" is activating`,
		},
		{
			name:        "inactive with multi-line output",
			installed:   []string{"systemctl"},
			results:     map[string]fakeResult{isActive: {out: "inactive\nWarning: unit changed on disk\n", err: errors.New("exit status 3")}},
			wantStatus:  StatusFail,
			wantMessage: `service "nginx" is not active (inactive)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, _ := fakeToolOptions(t, tt.installed, tt.results)
			opts.ServiceName = " nginx "
			assertOutcome(t, checkServiceUptime(opts), keyServiceUptime, tt.wantStatus, tt.wantMessage)
		})
	}
}

func TestCommandAppliesTimeout(t *testing.T) {
	var deadline time.Time
	opts := CheckOptions{
		CommandTimeout: 250 * time.Millisecond,
		runCommand: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			var ok bool
			if deadline, ok = ctx.Deadline(); !ok {
				t.Error("command context has no deadline")
			}
			return nil, nil
		},
	}

	before := time.Now()
	if _, err := opts.command("docker", "info"); err != nil {
		t.Fatalf("command() error = %v", err)
	}
	if remaining := deadline.Sub(before); remaining <= 0 || remaining > time.Second {
		t.Errorf("command deadline is %v after start, want about 250ms", remaining)
	}
}

func TestCheckMisconfigurationPassesInCleanEnvironment(t *testing.T) {
	isolateMisconfigEnv(t)

	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusPass, "no common misconfigurations detected")
}

func TestCheckMisconfigurationWarnsWithoutHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME is not required on Windows")
	}
	isolateMisconfigEnv(t)
	unsetEnv(t, "HOME")

	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, "HOME environment variable is not set")
}

func TestCheckMisconfigurationDatabasePermissions(t *testing.T) {
	isolateMisconfigEnv(t)

	tests := []struct {
		name string
		mode os.FileMode
		warn bool
	}{
		{name: "owner only", mode: 0o600, warn: false},
		{name: "group readable", mode: 0o640, warn: true},
		{name: "group and other writable", mode: 0o666, warn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "salus.db")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatalf("create database file: %v", err)
			}
			if err := os.Chmod(path, tt.mode); err != nil {
				t.Fatalf("chmod database file: %v", err)
			}
			t.Setenv(DatabasePathEnv, path)

			got := checkMisconfiguration(CheckOptions{})

			// Windows has no POSIX mode bits (Go reports 0666 for any writable
			// file), so the permission test is skipped there and never warns.
			wantWarn := tt.warn && runtime.GOOS != "windows"
			if wantWarn {
				want := fmt.Sprintf("%s is accessible by group/other (mode %04o, want 0600; run chmod 600 on it)", path, tt.mode)
				assertOutcome(t, got, keyMisconfig, StatusWarn, want)
				return
			}
			assertOutcome(t, got, keyMisconfig, StatusPass, "no common misconfigurations detected")
		})
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{name: "first of several lines", output: "  one\ntwo\n", want: "one"},
		{name: "error when output empty", output: " \n", err: errors.New("boom"), want: "boom"},
		{name: "fallback", want: "unknown error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstLine(tt.output, tt.err); got != tt.want {
				t.Errorf("firstLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCheckMisconfigurationChecksDefaultDatabasePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not checked on Windows")
	}
	isolateMisconfigEnv(t)

	path, err := DefaultDatabasePath()
	if err != nil {
		t.Fatalf("DefaultDatabasePath() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("create database file: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod database file: %v", err)
	}

	want := path + " is accessible by group/other (mode 0644, want 0600; run chmod 600 on it)"
	assertOutcome(t, checkMisconfiguration(CheckOptions{}), keyMisconfig, StatusWarn, want)
}

func TestValidUnitName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"nginx", true},
		{"nginx.service", true},
		{"getty@tty1.service", true},
		{`systemd-fsck@dev-disk-by\x2duuid.service`, true},
		{"--host=user@example.invalid", false},
		{"-H", false},
		{"", false},
		{"nginx;reboot", false},
		{"two words", false},
		{"$(id)", false},
		{strings.Repeat("a", 256), false},
	}

	for _, tt := range tests {
		if got := validUnitName(tt.name); got != tt.valid {
			t.Errorf("validUnitName(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

func TestCheckServiceUptimeRejectsInvalidNames(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("service names are only used on Linux")
	}

	for _, name := range []string{"--host=user@example.invalid", "-H", "nginx;reboot"} {
		t.Run(name, func(t *testing.T) {
			opts, calls := fakeToolOptions(t, []string{"systemctl"}, nil)
			opts.ServiceName = name

			want := fmt.Sprintf("invalid service name %q: use a systemd unit name such as nginx or nginx.service", name)
			assertOutcome(t, checkServiceUptime(opts), keyServiceUptime, StatusFail, want)
			if len(*calls) != 0 {
				t.Errorf("ran %q for an invalid service name", *calls)
			}
		})
	}
}
