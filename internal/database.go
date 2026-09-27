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
	"errors"
	"fmt"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	// DatabasePathEnv is the environment variable that sets the sqlite database path.
	DatabasePathEnv = "SALUS_DB_PATH"
	// DefaultDatabasePath is used when DatabasePathEnv is unset or empty.
	DefaultDatabasePath = "salus.db"
)

//--------------------------------------------------core-------------------------------------------------------------------------------------------------//

// Database owns the gorm connection for internal data access.
type Database struct {
	conn *gorm.DB
}

// NewDatabase opens (or creates) the sqlite file and runs schema migrations.
func NewDatabase(path string) (*Database, error) {
	if path == "" {
		path = DefaultDatabasePath
	}

	// GORM's default logger writes to stdout (corrupting --json output) and
	// logs "record not found" for normal lookups. Errors are returned to
	// callers and reported by the CLI instead.
	conn, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := conn.AutoMigrate(
		&FeatureCategory{},
		&Feature{},
		&ScanJob{},
		&ScanResult{},
	); err != nil {
		db := &Database{conn: conn}
		return nil, errors.Join(fmt.Errorf("auto-migrate schema: %w", err), db.Close())
	}

	return &Database{conn: conn}, nil
}

// Conn exposes the raw gorm handle for advanced queries/transactions.
func (d *Database) Conn() *gorm.DB {
	return d.conn
}

// Close releases the underlying connection pool. Windows cannot delete the
// database file while it is open, so tests and main must always call it.
func (d *Database) Close() error {
	sqlDB, err := d.conn.DB()
	if err != nil {
		return fmt.Errorf("get sql.DB: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}
	return nil
}

//-----------------------------------------------------------models and types------------------------------------------------------------------------------------------------//

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("record not found")

// FeatureCategory groups related health checks together (e.g. "System Resources").
type FeatureCategory struct {
	ID          uint   `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex;not null"`
	Description string
}

// Feature describes a single health check available to run.
type Feature struct {
	ID          uint `gorm:"primaryKey"`
	CategoryID  uint `gorm:"not null;index"`
	Category    FeatureCategory
	Key         string `gorm:"uniqueIndex;not null"`
	Name        string `gorm:"not null"`
	Description string
}

// ScanJob records a single execution of one or more health checks.
type ScanJob struct {
	ID         uint `gorm:"primaryKey"`
	StartedAt  time.Time
	FinishedAt *time.Time
	Status     string // "running", "completed", or "failed"
	Summary    string
}

// ScanResult records the outcome of one health check within a ScanJob.
type ScanResult struct {
	ID         uint `gorm:"primaryKey"`
	ScanJobID  uint `gorm:"not null;index"`
	FeatureID  uint `gorm:"not null;index"`
	Feature    Feature
	Key        string `gorm:"not null"`
	Status     string `gorm:"not null"` // "PASS", "WARN", or "FAIL"
	Message    string
	DurationMs int64
	CreatedAt  time.Time
}
