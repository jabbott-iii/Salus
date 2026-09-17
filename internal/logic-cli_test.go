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
	"io"
	"testing"
)

type failingWriter struct {
	err error
}

func (w failingWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

func TestCheckListCmdReturnsWriteError(t *testing.T) {
	db := newSeededTestDatabase(t)
	expectedErr := errors.New("write failed")

	cmd := newCheckListCmd(db)
	cmd.SilenceUsage = true
	cmd.SetOut(failingWriter{err: expectedErr})
	cmd.SetErr(io.Discard)

	err := cmd.Execute()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("Execute() error = %v, want %v", err, expectedErr)
	}
}
