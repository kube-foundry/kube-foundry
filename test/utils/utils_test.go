/*
Copyright 2026.

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

package utils

import (
	"strings"
	"testing"
)

func TestGetNonEmptyLines_Basic(t *testing.T) {
	lines := GetNonEmptyLines("foo\nbar\nbaz")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "foo" || lines[1] != "bar" || lines[2] != "baz" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestGetNonEmptyLines_WithEmptyLines(t *testing.T) {
	lines := GetNonEmptyLines("foo\n\nbar\n\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "foo" || lines[1] != "bar" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestGetNonEmptyLines_Empty(t *testing.T) {
	lines := GetNonEmptyLines("")
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}

func TestGetNonEmptyLines_SingleLine(t *testing.T) {
	lines := GetNonEmptyLines("hello")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lines[0] != "hello" {
		t.Errorf("expected 'hello', got %q", lines[0])
	}
}

func TestGetNonEmptyLines_AllEmpty(t *testing.T) {
	lines := GetNonEmptyLines("\n\n\n")
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}

func TestGetProjectDir_StripsSuffix(t *testing.T) {
	dir, err := GetProjectDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dir == "" {
		t.Fatal("expected non-empty directory")
	}
	// Should not contain the test/e2e suffix since GetProjectDir strips it
	if strings.HasSuffix(dir, "/test/e2e") {
		t.Errorf("expected /test/e2e to be stripped, got %q", dir)
	}
}
