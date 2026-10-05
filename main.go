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

package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/jabbott-iii/Salus/pkg"
	"github.com/spf13/cobra"
)

// version is reported by --version. Release builds set it with
// -ldflags "-X main.version=vX.Y.Z" (see .github/workflows/cd.yml).
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI and returns the process exit code: 0/1/2 for a check
// run's PASS/WARN/FAIL result, and pkg.ExitCodeError for operational
// errors. Returning instead of exiting lets the database close first.
func run(args []string, stdout, stderr io.Writer) int {
	return runWith(newRootCmd, args, stdout, stderr)
}

// newRootCmd builds the Salus command tree with the build version attached,
// which makes Cobra provide the --version flag.
func newRootCmd(openDB pkg.DatabaseOpener) *cobra.Command {
	return pkg.NewVersionedRootCmd(openDB, version)
}

// runWith is run with the command tree's constructor as a parameter, so
// tests can exercise the panic handling.
func runWith(build func(pkg.DatabaseOpener) *cobra.Command, args []string, stdout, stderr io.Writer) (code int) {
	// The database is opened only when a command needs it, at most once.
	var db *pkg.Database
	openDB := func() (*pkg.Database, error) {
		if db != nil {
			return db, nil
		}
		path, err := pkg.DatabasePath()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		opened, err := pkg.OpenDatabase(path)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		db = opened
		return db, nil
	}
	defer func() {
		if db == nil {
			return
		}
		if err := db.Close(); err != nil {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
			if code == pkg.ExitCodePass {
				code = pkg.ExitCodeError
			}
		}
	}()

	// A panic is a bug in Salus, not a check result. Without this, Go would
	// exit with 2, the code for a FAIL result. Registered after the database
	// close above, so it runs first and the database still closes.
	defer func() {
		if r := recover(); r != nil {
			_, _ = fmt.Fprintf(stderr, "Error: internal error: %v\n%s", r, debug.Stack())
			code = pkg.ExitCodeError
		}
	}()

	rootCmd := build(openDB)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	cmd, err := rootCmd.ExecuteC()
	code = pkg.ExitCode(err)
	if code == pkg.ExitCodeError {
		// The command tree silences Cobra's own error printing, so errors are
		// reported here, on stderr only, and never mix into stdout or --json.
		if cmd == nil {
			cmd = rootCmd
		}
		_, _ = fmt.Fprintf(stderr, "Error: %v\nRun '%s --help' for usage.\n", err, cmd.CommandPath())
	}
	return code
}
