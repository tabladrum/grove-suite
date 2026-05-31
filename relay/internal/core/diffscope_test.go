package core

import (
	"reflect"
	"testing"
)

// Sanity covers the parser's claim that:
//   - hunk headers anchor the +line counter correctly,
//   - `-` lines don't advance the new-file counter,
//   - `+++ /dev/null` is dropped (file deletions don't count as new),
//   - multiple hunks in one file merge into ascending non-overlapping ranges.
func TestParseAddedLines_BasicHunk(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 line1
+added2
 line2
 line3
@@ -10,2 +11,3 @@
 ctx10
+addedAt12
 ctx11
`
	got := ParseAddedLines(diff)
	want := AddedLineMap{
		"foo.go": {{Start: 2, End: 3}, {Start: 12, End: 13}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestParseAddedLines_NewFile(t *testing.T) {
	diff := `--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package new
+
+func A() {}
`
	got := ParseAddedLines(diff)
	rs, ok := got["new.go"]
	if !ok {
		t.Fatalf("expected new.go in map: %#v", got)
	}
	if len(rs) != 1 || rs[0] != (LineRange{Start: 1, End: 4}) {
		t.Fatalf("unexpected range: %#v", rs)
	}
}

func TestParseAddedLines_Deletion(t *testing.T) {
	diff := `--- a/gone.go
+++ /dev/null
@@ -1,2 +0,0 @@
-package gone
-
`
	got := ParseAddedLines(diff)
	if len(got) != 0 {
		t.Fatalf("deletion should produce empty map, got %#v", got)
	}
}

func TestAddedLineMap_InScope(t *testing.T) {
	m := AddedLineMap{
		"a.go": {{Start: 10, End: 15}},
	}
	cases := []struct {
		path string
		line int
		want bool
	}{
		{"a.go", 10, true},
		{"a.go", 14, true},
		{"a.go", 15, false},
		{"a.go", 9, false},
		{"b.go", 10, false},
		{"a.go", 0, true},    // line=0 → file-level finding, in-scope if file touched
		{"./a.go", 12, true}, // normalization
	}
	for _, c := range cases {
		if got := m.InScope(c.path, c.line); got != c.want {
			t.Errorf("InScope(%q,%d)=%v want %v", c.path, c.line, got, c.want)
		}
	}
}
