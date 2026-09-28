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
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func newSeededTestDatabase(t *testing.T) *Database {
	t.Helper()

	db := newTestDatabase(t)
	if err := EnsureDefaultFeatures(db); err != nil {
		t.Fatalf("EnsureDefaultFeatures() error = %v", err)
	}
	return db
}

func TestRecordScanPersistsJobAndResults(t *testing.T) {
	db := newSeededTestDatabase(t)

	outcomes := []CheckOutcome{
		{Key: keyDiskSpace, Status: StatusPass, Message: "fine", Duration: 5 * time.Millisecond},
		{Key: keyMemory, Status: StatusWarn, Message: "getting full", Duration: 10 * time.Millisecond},
	}

	job, err := RecordScan(db, time.Time{}, outcomes)
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	if job.ID == 0 {
		t.Fatal("RecordScan() returned job with zero ID")
	}
	if job.Status != "completed" {
		t.Errorf("job.Status = %q, want %q", job.Status, "completed")
	}
	if job.Summary != "1 pass, 1 warn, 0 fail" {
		t.Errorf("job.Summary = %q, want %q", job.Summary, "1 pass, 1 warn, 0 fail")
	}

	gotJob, results, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}
	if gotJob.ID != job.ID {
		t.Errorf("GetScanJob() ID = %d, want %d", gotJob.ID, job.ID)
	}
	if len(results) != len(outcomes) {
		t.Fatalf("GetScanJob() returned %d results, want %d", len(results), len(outcomes))
	}
	if results[0].Key != keyDiskSpace || results[0].Status != string(StatusPass) {
		t.Errorf("results[0] = %+v, want key %q status %q", results[0], keyDiskSpace, StatusPass)
	}
}

func TestRecordScanUnknownFeatureFails(t *testing.T) {
	db := newSeededTestDatabase(t)

	outcomes := []CheckOutcome{{Key: "not-a-real-check", Status: StatusPass}}
	if _, err := RecordScan(db, time.Time{}, outcomes); err == nil {
		t.Fatal("RecordScan() expected error for unknown feature key, got nil")
	}
}

func TestGetScanJobNotFound(t *testing.T) {
	db := newSeededTestDatabase(t)

	if _, _, err := GetScanJob(db, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetScanJob() error = %v, want ErrNotFound", err)
	}
}

func TestListScanJobsOrdersNewestFirst(t *testing.T) {
	db := newSeededTestDatabase(t)

	first, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	second, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusPass}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}

	jobs, err := ListScanJobs(db, 0)
	if err != nil {
		t.Fatalf("ListScanJobs() error = %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("ListScanJobs() returned %d jobs, want 2", len(jobs))
	}
	if jobs[0].ID != second.ID || jobs[1].ID != first.ID {
		t.Errorf("ListScanJobs() order = [%d, %d], want [%d, %d]", jobs[0].ID, jobs[1].ID, second.ID, first.ID)
	}

	limited, err := ListScanJobs(db, 1)
	if err != nil {
		t.Fatalf("ListScanJobs(limit=1) error = %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("ListScanJobs(limit=1) returned %d jobs, want 1", len(limited))
	}
}

func TestRecordScanBoundsStoredMessages(t *testing.T) {
	db := newSeededTestDatabase(t)
	long := strings.Repeat("é", maxStoredMessageLen) // 2 bytes per rune

	job, err := RecordScan(db, time.Time{}, []CheckOutcome{{Key: keyMisconfig, Status: StatusWarn, Message: long}})
	if err != nil {
		t.Fatalf("RecordScan() error = %v", err)
	}
	_, results, err := GetScanJob(db, job.ID)
	if err != nil {
		t.Fatalf("GetScanJob() error = %v", err)
	}

	got := results[0].Message
	if len(got) > maxStoredMessageLen || !utf8.ValidString(got) || !strings.HasSuffix(got, "...") {
		t.Errorf("stored message: %d bytes, valid UTF-8 %v, suffix %q; want <= %d bytes, valid, ending in ...",
			len(got), utf8.ValidString(got), got[max(0, len(got)-3):], maxStoredMessageLen)
	}
}

func TestTruncateMessage(t *testing.T) {
	tests := []struct {
		msg  string
		max  int
		want string
	}{
		{msg: "short", max: 10, want: "short"},
		{msg: "exactly10!", max: 10, want: "exactly10!"},
		{msg: "0123456789abc", max: 10, want: "0123456..."},
		{msg: "ééééé", max: 8, want: "éé..."}, // never splits a multi-byte rune
	}

	for _, tt := range tests {
		if got := truncateMessage(tt.msg, tt.max); got != tt.want {
			t.Errorf("truncateMessage(%q, %d) = %q, want %q", tt.msg, tt.max, got, tt.want)
		}
	}
}
