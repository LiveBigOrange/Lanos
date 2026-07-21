package transfer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeRelativePathStripsDriveLetter(t *testing.T) {
	cases := map[string]string{
		`C:\Users\foo\bar.txt`: "Users/foo/bar.txt",
		`D:/data/report.pdf`:   "data/report.pdf",
		`c:\a\b`:               "a/b",
		`/home/user/file.txt`:  "home/user/file.txt",
		`relative/path/file`:   "relative/path/file",
		`file.txt`:             "file.txt",
		`C:\`:                  "",
		`C:`:                   "",
	}
	for in, want := range cases {
		got := SanitizeRelativePath(in)
		if got != want {
			t.Errorf("SanitizeRelativePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeRelativePathStripsUNC(t *testing.T) {
	cases := map[string]string{
		`\\server\share\dir\file.txt`: "dir/file.txt",
		`//server/share/file.txt`:     "file.txt",
		`\\server\share\`:             "",
	}
	for in, want := range cases {
		got := SanitizeRelativePath(in)
		if got != want {
			t.Errorf("SanitizeRelativePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeRelativePathReplacesIllegalChars(t *testing.T) {
	cases := map[string]string{
		`a<b>c.txt`:         "a_b_c.txt",
		`foo:bar`:           "foo_bar",
		`qu"ote".`:          "qu_ote_",
		`a|b?c*d`:           "a_b_c_d",
		"con\x00trol":       "con_trol",
		`path\with\slashes`: "path/with/slashes",
	}
	for in, want := range cases {
		got := SanitizeRelativePath(in)
		if got != want {
			t.Errorf("SanitizeRelativePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeRelativePathNeutralizesTraversal(t *testing.T) {
	cases := map[string]string{
		`../../../etc/passwd`: "etc/passwd",
		`a/../../b`:           "a/b",
		`..\..\secret`:        "secret",
		`foo/./bar/..`:        "foo/bar",
		`..`:                  "",
		`./..`:                "",
	}
	for in, want := range cases {
		got := SanitizeRelativePath(in)
		if got != want {
			t.Errorf("SanitizeRelativePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeRelativePathTrimsTrailingDots(t *testing.T) {
	cases := map[string]string{
		`file.txt.`: "file.txt",
		`folder.`:   "folder",
		`a.b.c.`:    "a.b.c",
		`name `:     "name",
	}
	for in, want := range cases {
		got := SanitizeRelativePath(in)
		if got != want {
			t.Errorf("SanitizeRelativePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinSavePathBasic(t *testing.T) {
	dir := t.TempDir()
	got, err := JoinSavePath(dir, "sub/dir/file.txt", "file.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "sub", "dir", "file.txt")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestJoinSavePathTraversalContained(t *testing.T) {
	dir := t.TempDir()
	// "../.." is dropped, leaving "etc/passwd" inside dir -> contained, no error.
	got, err := JoinSavePath(dir, "../../../etc/passwd", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isWithin(got, dir) {
		t.Fatalf("path escaped save dir: %q", got)
	}
	if filepath.Base(got) != "passwd" {
		t.Fatalf("got leaf %q", filepath.Base(got))
	}
}

func TestJoinSavePathAllTraversalUsesBaseName(t *testing.T) {
	dir := t.TempDir()
	got, err := JoinSavePath(dir, "..", "fallback.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(got) != "fallback.txt" {
		t.Fatalf("expected fallback name, got %q", filepath.Base(got))
	}
	if !isWithin(got, dir) {
		t.Fatalf("path escaped save dir: %q", got)
	}
}

func TestJoinSavePathDeConflicts(t *testing.T) {
	dir := t.TempDir()
	first, err := JoinSavePath(dir, "file.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(first), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := JoinSavePath(dir, "file.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(second) != "file (1).txt" {
		t.Fatalf("expected de-conflicted name, got %q", filepath.Base(second))
	}
}

func TestJoinSavePathEmptyUsesBaseName(t *testing.T) {
	dir := t.TempDir()
	got, err := JoinSavePath(dir, "", "report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "report.pdf" {
		t.Fatalf("got %q", filepath.Base(got))
	}
}

func TestJoinSavePathAllEmptyUsesUnnamed(t *testing.T) {
	dir := t.TempDir()
	got, err := JoinSavePath(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "unnamed" {
		t.Fatalf("got %q", filepath.Base(got))
	}
}

func TestJoinSavePathCrossOS(t *testing.T) {
	dir := t.TempDir()
	got, err := JoinSavePath(dir, `C:\Users\Alice\Docs\report.pdf`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !isWithin(got, dir) {
		t.Fatalf("path escaped: %q", got)
	}
	if filepath.Base(got) != "report.pdf" {
		t.Fatalf("got leaf %q", filepath.Base(got))
	}
	if err := os.MkdirAll(filepath.Dir(got), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(got, []byte("x"), 0o644); err != nil {
		t.Fatalf("cannot write cross-OS path: %v", err)
	}
}

func TestIsValidUTF8(t *testing.T) {
	cases := map[string]bool{
		"hello":           true,
		"héllo":           true,
		"日本語":             true,
		"bad\xff\xfeutf8": false,
		"with\x00null":    false,
	}
	for in, want := range cases {
		if got := IsValidUTF8(in); got != want {
			t.Errorf("IsValidUTF8(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestStripDriveOrUNC(t *testing.T) {
	cases := map[string]string{
		`C:\foo`:          "foo",
		`D:/bar`:          "bar",
		`\\srv\share\baz`: "baz",
		`//srv/share/qux`: "qux",
		`/no/drive`:       "/no/drive",
		`plain`:           "plain",
	}
	for in, want := range cases {
		got := stripDriveOrUNC(in)
		if got != want {
			t.Errorf("stripDriveOrUNC(%q) = %q, want %q", in, got, want)
		}
	}
}

func errIsPathEscape(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "escapes save directory")
}
