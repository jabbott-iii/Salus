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
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

	// KubeContext selects a kubeconfig context for kubectl; empty means the
	// current context.
	KubeContext string

	CommandTimeout time.Duration

	// Test seams for external tools; nil means exec.CommandContext(...).CombinedOutput
	// and exec.LookPath. Tests set them so no real docker/kubectl/systemctl runs.
	runCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookPath   func(file string) (string, error)
}

// Defaults applied when the corresponding CheckOptions field is unset (<= 0).
// check run also uses them as its flag defaults, on every platform. The
// resource checks read thresholds through the accessors in health-thresholds.go.
const (
	defaultDiskWarnPercent = 80.0
	defaultDiskFailPercent = 90.0
	defaultMemWarnPercent  = 80.0
	defaultMemFailPercent  = 90.0
	defaultLoadWarnPercent = 80.0
	defaultLoadFailPercent = 100.0
	defaultCommandTimeout  = 3 * time.Second
)

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

// klogLine matches the log lines kubectl writes to stderr, such as
// "E0928 12:00:00.000000   12345 memcache.go:265] ...".
var klogLine = regexp.MustCompile(`^[IWEF]\d{4} \d{2}:\d{2}:\d{2}\.\d+\s`)

// errorLine picks the line of a failed tool's combined output that states the
// error. It skips Docker CLI warnings, kubectl log lines, and the hint that
// kubectl cluster-info prints on every run, and otherwise behaves like
// firstLine.
func errorLine(output string, err error) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "WARNING") || klogLine.MatchString(line) ||
			strings.HasPrefix(line, "To further debug and diagnose cluster problems") {
			continue
		}
		return line
	}
	return firstLine(output, err)
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

// ValidateCheckKeys returns an error for the first key that is not a built-in check.
func ValidateCheckKeys(keys []string) error {
	for _, key := range keys {
		if _, ok := checkRegistry[key]; !ok {
			return fmt.Errorf("unknown check %q", key)
		}
	}
	return nil
}

// RunChecks executes the given check keys (or all built-in checks when keys is empty)
// and returns their outcomes in the order requested.
func RunChecks(keys []string, opts CheckOptions) ([]CheckOutcome, error) {
	if len(keys) == 0 {
		keys = AllCheckKeys
	}
	if err := ValidateCheckKeys(keys); err != nil {
		return nil, err
	}

	outcomes := make([]CheckOutcome, 0, len(keys))
	for _, key := range keys {
		outcome := checkRegistry[key](opts)
		outcome.Message = sanitizeMessage(outcome.Message)
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

// sanitizeMessage replaces control characters with '?'. Messages embed
// external tool output, which a hostile Docker daemon or Kubernetes API server
// could fill with terminal escape sequences; after this, a message can neither
// drive the terminal nor span lines (SEC-009).
func sanitizeMessage(msg string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, msg)
}

//--------------------------------------------------container & orchestration checks-------------------------------------------------------------------//

func checkDockerStatus(opts CheckOptions) CheckOutcome {
	start := time.Now()

	if !opts.hasTool("docker") {
		return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: "docker CLI not found in PATH", Duration: time.Since(start)}
	}

	out, err := opts.command("docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil {
		return CheckOutcome{Key: keyDocker, Status: StatusFail, Message: fmt.Sprintf("docker daemon unreachable: %s", errorLine(string(out), err)), Duration: time.Since(start)}
	}
	msg := fmt.Sprintf("docker daemon reachable (server version %s)", dockerServerVersion(string(out)))

	// A reachable daemon can still run broken workloads. Containers failing
	// their HEALTHCHECK or restarting are a WARN: the runtime itself works.
	var problems []string
	for _, query := range []struct {
		label string
		args  []string
	}{
		{"unhealthy", []string{"ps", "--filter", "health=unhealthy", "--format", "{{.Names}}"}},
		{"restarting", []string{"ps", "--all", "--filter", "status=restarting", "--format", "{{.Names}}"}},
	} {
		out, err := opts.command("docker", query.args...)
		if err != nil {
			return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: fmt.Sprintf("%s; container health unknown: %s", msg, errorLine(string(out), err)), Duration: time.Since(start)}
		}
		if names := containerNames(string(out)); len(names) > 0 {
			problems = append(problems, query.label+": "+nameList(names))
		}
	}

	if len(problems) > 0 {
		return CheckOutcome{Key: keyDocker, Status: StatusWarn, Message: msg + "; " + strings.Join(problems, "; "), Duration: time.Since(start)}
	}
	return CheckOutcome{Key: keyDocker, Status: StatusPass, Message: msg, Duration: time.Since(start)}
}

// dockerServerVersion returns the version from docker info output. The output
// is combined with stderr, so blank and WARNING lines are skipped.
func dockerServerVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "WARNING") {
			return line
		}
	}
	return "unknown"
}

// containerNamePattern matches one line of docker ps --format {{.Names}}
// output; legacy links add comma-separated names with slashes.
var containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.,/-]*$`)

// containerNames returns the container names in docker ps output, ignoring
// warnings the docker CLI may print to stderr.
func containerNames(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); containerNamePattern.MatchString(line) {
			names = append(names, line)
		}
	}
	return names
}

// maxListedNames bounds how many names one message lists.
const maxListedNames = 5

// nameList joins names for a single-line message, listing at most
// maxListedNames and counting the rest.
func nameList(names []string) string {
	if len(names) <= maxListedNames {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxListedNames], ", "), len(names)-maxListedNames)
}

// nodeReadinessJSONPath prints one "name<TAB>Ready condition status" line per
// node.
const nodeReadinessJSONPath = `jsonpath={range .items[*]}{.metadata.name}{"\t"}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}`

func checkKubernetesStatus(opts CheckOptions) CheckOutcome {
	start := time.Now()
	kubeContext := strings.TrimSpace(opts.KubeContext)

	if kubeContext != "" && !validKubeContext(kubeContext) {
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("invalid kubeconfig context %q: it must not start with - or contain control characters", kubeContext), Duration: time.Since(start)}
	}

	if !opts.hasTool("kubectl") {
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: "kubectl CLI not found in PATH", Duration: time.Since(start)}
	}

	// A single "--context=<name>" argument binds the value to the flag, so the
	// name can never be read as another kubectl option.
	kubectl := func(args ...string) ([]byte, error) {
		if kubeContext != "" {
			args = append([]string{"--context=" + kubeContext}, args...)
		}
		return opts.command("kubectl", args...)
	}
	cluster := "kubernetes cluster"
	if kubeContext != "" {
		cluster = fmt.Sprintf("kubernetes cluster (context %s)", kubeContext)
	}

	// cluster-info lists Services in kube-system, which namespace-scoped users
	// may not do. A Forbidden answer still proves the API server is reachable.
	out, err := kubectl("cluster-info")
	if err != nil && !forbidden(string(out)) {
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("%s unreachable: %s", cluster, errorLine(string(out), err)), Duration: time.Since(start)}
	}

	out, err = kubectl("get", "nodes", "-o", nodeReadinessJSONPath)
	if err != nil {
		if forbidden(string(out)) {
			return CheckOutcome{Key: keyKubernetes, Status: StatusPass, Message: fmt.Sprintf("%s reachable; node readiness not checked (listing nodes is forbidden)", cluster), Duration: time.Since(start)}
		}
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: fmt.Sprintf("%s reachable; node readiness unknown: %s", cluster, errorLine(string(out), err)), Duration: time.Since(start)}
	}

	ready, notReady := parseNodeReadiness(string(out))
	total := ready + len(notReady)
	switch {
	case total == 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("%s reachable, but it has no nodes", cluster), Duration: time.Since(start)}
	case ready == 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusFail, Message: fmt.Sprintf("%s reachable, but no nodes are Ready (0/%d); NotReady: %s", cluster, total, nameList(notReady)), Duration: time.Since(start)}
	case len(notReady) > 0:
		return CheckOutcome{Key: keyKubernetes, Status: StatusWarn, Message: fmt.Sprintf("%s reachable; %d/%d nodes Ready; NotReady: %s", cluster, ready, total, nameList(notReady)), Duration: time.Since(start)}
	default:
		return CheckOutcome{Key: keyKubernetes, Status: StatusPass, Message: fmt.Sprintf("%s reachable; %d/%d nodes Ready", cluster, ready, total), Duration: time.Since(start)}
	}
}

// parseNodeReadiness counts Ready nodes and names the others in output
// produced with nodeReadinessJSONPath. Lines without a tab, such as kubectl
// warnings on stderr, are ignored.
func parseNodeReadiness(out string) (ready int, notReady []string) {
	for _, line := range strings.Split(out, "\n") {
		name, status, ok := strings.Cut(strings.TrimRight(line, "\r"), "\t")
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		if status == "True" {
			ready++
		} else {
			notReady = append(notReady, name)
		}
	}
	return ready, notReady
}

// forbidden reports whether kubectl output carries an HTTP 403 from the API
// server, as in `Error from server (Forbidden): nodes is forbidden: ...`.
func forbidden(output string) bool {
	return strings.Contains(output, "(Forbidden)") || strings.Contains(output, " is forbidden:")
}

// validKubeContext reports whether name is an acceptable --kube-context value:
// valid UTF-8 of at most 253 bytes, with no control characters, and not
// starting with "-". Kubeconfig context names have no fixed character set,
// such as arn:aws:eks:region:account:cluster/name or "prod (eu)". The name is
// passed as the single argument --context=<name>, so it cannot become
// another kubectl option.
func validKubeContext(name string) bool {
	return len(name) <= 253 && utf8.ValidString(name) && !strings.HasPrefix(name, "-") &&
		!strings.ContainsFunc(name, unicode.IsControl)
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

	if !validUnitName(name) {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusFail, Message: fmt.Sprintf("invalid service name %q: use a systemd unit name such as nginx or nginx.service", name), Duration: time.Since(start)}
	}

	if !opts.hasTool("systemctl") {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "systemctl not found in PATH", Duration: time.Since(start)}
	}

	// "--" ends option parsing, so the name can never be read as a systemctl flag.
	out, err := opts.command("systemctl", "is-active", "--", name)
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

// unitNamePattern matches systemd unit names: letters, digits, and ":-_.\@",
// not starting with "-" (which systemctl would parse as an option; SEC-001).
var unitNamePattern = regexp.MustCompile(`^[A-Za-z0-9:_.\\@][A-Za-z0-9:_.\\@-]*$`)

// validUnitName reports whether name is an acceptable --service value.
func validUnitName(name string) bool {
	return len(name) <= 255 && unitNamePattern.MatchString(name)
}

func checkProcessUptime(start time.Time) CheckOutcome {
	uptime, err := readSystemUptime()
	if err != nil {
		return CheckOutcome{Key: keyServiceUptime, Status: StatusWarn, Message: "no --service provided and host uptime unavailable: " + err.Error(), Duration: time.Since(start)}
	}
	return CheckOutcome{Key: keyServiceUptime, Status: StatusPass, Message: fmt.Sprintf("host has been up for %s", uptime.Round(time.Second)), Duration: time.Since(start)}
}

// misconfigRule is one misconfiguration test. Its id prefixes every problem
// it reports, so scripts can match problems by a stable identifier.
type misconfigRule struct {
	id    string
	check func() []string
}

// misconfigRules run in this order. The ids are a documented contract. The
// permission rules use POSIX mode bits and skip Windows, where Go reports 0666
// for every writable file (ACLs are not mode bits).
var misconfigRules = []misconfigRule{
	{"home-unset", homeUnset},
	{"db-permissions", databasePermissions},
	{"kubeconfig-permissions", kubeconfigPermissions},
	{"docker-socket-permissions", dockerSocketPermissions},
	{"path-world-writable", worldWritablePath},
}

func checkMisconfiguration(opts CheckOptions) CheckOutcome {
	start := time.Now()
	var warnings []string

	for _, rule := range misconfigRules {
		for _, problem := range rule.check() {
			warnings = append(warnings, rule.id+": "+problem)
		}
	}

	if len(warnings) == 0 {
		return CheckOutcome{Key: keyMisconfig, Status: StatusPass, Message: "no common misconfigurations detected", Duration: time.Since(start)}
	}

	return CheckOutcome{Key: keyMisconfig, Status: StatusWarn, Message: strings.Join(warnings, "; "), Duration: time.Since(start)}
}

func homeUnset() []string {
	if _, ok := os.LookupEnv("HOME"); !ok && runtime.GOOS != "windows" {
		return []string{"HOME environment variable is not set"}
	}
	return nil
}

// databasePermissions checks the database Salus actually uses (SALUS_DB_PATH
// or the per-user default).
func databasePermissions() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	dbPath, err := DatabasePath()
	if err != nil {
		return nil
	}
	file, ok := databaseFile(dbPath)
	if !ok {
		return nil
	}
	return ownerOnly(file)
}

// kubeconfigPermissions checks the files kubectl reads, which hold cluster
// credentials: the KUBECONFIG list, or ~/.kube/config.
func kubeconfigPermissions() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	paths := filepath.SplitList(os.Getenv("KUBECONFIG"))
	if len(paths) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		paths = []string{filepath.Join(home, ".kube", "config")}
	}

	var problems []string
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		problems = append(problems, ownerOnly(path)...)
	}
	return problems
}

// ownerOnly reports an existing file that grants any group/other permission,
// unless its mode bits are made up (WSL drvfs; see syntheticModes).
func ownerOnly(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 && !syntheticModes(path) {
		return []string{fmt.Sprintf("%s is accessible by group/other (mode %04o, want 0600; run chmod 600 on it)", path, perm)}
	}
	return nil
}

// dockerSocketPermissions warns when every local user can write to the Docker
// socket the CLI uses, which gives them root-equivalent control of the host.
// Access for the docker group is normal.
func dockerSocketPermissions() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	path, ok := dockerSocketPath()
	if !ok {
		return nil // a remote daemon has no local socket
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if perm := info.Mode().Perm(); perm&0o002 != 0 {
		return []string{fmt.Sprintf("%s is writable by all users (mode %04o), which gives every local user control of Docker", path, perm)}
	}
	return nil
}

// dockerSocketPath returns the local socket named by DOCKER_HOST (unix://),
// or the default socket. It reports false for a remote daemon. Docker
// contexts, which can select another endpoint, are not consulted.
func dockerSocketPath() (string, bool) {
	const defaultSocket = "/var/run/docker.sock"
	host := os.Getenv("DOCKER_HOST")
	path, ok := strings.CutPrefix(host, "unix://")
	switch {
	case host == "" || (ok && path == ""):
		return defaultSocket, true
	case !ok:
		return "", false
	default:
		return path, true
	}
}

// worldWritablePath warns about PATH directories that every user can write
// to: anyone could add a program there that shadows a trusted one. An empty
// entry means the current directory. WSL drvfs directories (the Windows PATH
// that WSL appends) report a made-up 0777 and are skipped.
func worldWritablePath() []string {
	if runtime.GOOS == "windows" {
		return nil
	}

	var dirs []string
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			dir = "."
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		if perm := info.Mode().Perm(); perm&0o002 != 0 && !syntheticModes(dir) {
			dirs = append(dirs, fmt.Sprintf("%s (mode %04o)", dir, perm))
		}
	}

	if len(dirs) == 0 {
		return nil
	}
	return []string{"PATH directories writable by all users: " + nameList(dirs)}
}
