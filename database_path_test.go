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
	"os"
	"path/filepath"
	"testing"

	"github.com/jabbott-iii/Salus/internal"
)

func TestDatabasePathFromEnvUsesConfiguredPath(t *testing.T) {
	want := filepath.Join(t.TempDir(), "tasks.db")
	t.Setenv(internal.DatabasePathEnv, want)

	if got := databasePathFromEnv(); got != want {
		t.Fatalf("databasePathFromEnv() = %q, want %q", got, want)
	}
}

func TestDatabasePathFromEnvUsesDefaultWhenUnset(t *testing.T) {
	previous, wasSet := os.LookupEnv(internal.DatabasePathEnv)
	if err := os.Unsetenv(internal.DatabasePathEnv); err != nil {
		t.Fatalf("unset %s: %v", internal.DatabasePathEnv, err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(internal.DatabasePathEnv, previous)
			return
		}
		_ = os.Unsetenv(internal.DatabasePathEnv)
	})

	if got := databasePathFromEnv(); got != internal.DefaultDatabasePath {
		t.Fatalf("databasePathFromEnv() = %q, want %q", got, internal.DefaultDatabasePath)
	}
}
