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

	"github.com/jabbott-iii/Salus/internal"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI and returns the process exit code: 0/1/2 for a check
// run's PASS/WARN/FAIL result, and internal.ExitCodeError for operational
// errors. Returning instead of exiting lets the database close first.
func run(args []string, stdout, stderr io.Writer) (code int) {
	// The database is opened only when a command needs it, at most once.
	var db *internal.Database
	openDB := func() (*internal.Database, error) {
		if db != nil {
			return db, nil
		}
		path, err := internal.DatabasePath()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database: %w", err)
		}
		opened, err := internal.OpenDatabase(path)
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
			if code == internal.ExitCodePass {
				code = internal.ExitCodeError
			}
		}
	}()

	rootCmd := newRootCmd(openDB)
	rootCmd.SetArgs(args)
	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	cmd, err := rootCmd.ExecuteC()
	code = internal.ExitCode(err)
	if code == internal.ExitCodeError {
		// The command tree silences Cobra's own error printing, so errors are
		// reported here, on stderr only, and never mix into stdout or --json.
		if cmd == nil {
			cmd = rootCmd
		}
		_, _ = fmt.Fprintf(stderr, "Error: %v\nRun '%s --help' for usage.\n", err, cmd.CommandPath())
	}
	return code
}
