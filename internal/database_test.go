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
	"testing"
)

func newTestDatabase(t *testing.T) *Database {
	t.Helper()

	db, err := NewDatabase(filepath.Join(t.TempDir(), "salus_test.db"))
	if err != nil {
		t.Fatalf("NewDatabase() error = %v", err)
	}
	// Registered after t.TempDir, so it runs first: Windows cannot remove an open file.
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return db
}

func TestNewDatabaseCreatesSchema(t *testing.T) {
	db := newTestDatabase(t)

	if err := db.Conn().AutoMigrate(&FeatureCategory{}, &Feature{}, &ScanJob{}, &ScanResult{}); err != nil {
		t.Fatalf("expected schema already migrated, got error: %v", err)
	}
}

func TestEnsureDefaultFeaturesSeedsCatalog(t *testing.T) {
	db := newTestDatabase(t)

	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() error = %v", err)
	}

	features, err := ListFeatures(db)
	if err != nil {
		t.Fatalf("ListFeatures() error = %v", err)
	}
	if len(features) != len(defaultFeatures) {
		t.Fatalf("ListFeatures() returned %d features, want %d", len(features), len(defaultFeatures))
	}

	// Seeding twice must be idempotent.
	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() second call error = %v", err)
	}
	features, err = ListFeatures(db)
	if err != nil {
		t.Fatalf("ListFeatures() error = %v", err)
	}
	if len(features) != len(defaultFeatures) {
		t.Fatalf("ListFeatures() after re-seed returned %d features, want %d", len(features), len(defaultFeatures))
	}
}
