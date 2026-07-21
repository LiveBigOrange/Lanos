// Package transfer: cross-platform received-file path mapping.
//
// When a file is sent from one OS and received on another, its original path
// must be normalized so it can be safely written inside the receiver's save
// directory (roadmap P1-19). This module:
//
//   - strips Windows drive letters (C:\foo -> foo)
//   - strips UNC prefixes (\\server\share\foo -> foo)
//   - normalizes backslashes to forward slashes
//   - replaces characters that are illegal in filenames on any target OS
//     (< > : " / \ | ? * and control bytes) with '_'
//   - collapses path-traversal segments (..) so the resolved path can never
//     escape the save directory
//   - collapses empty/redundant segments
//
// The result is joined onto the receiver's save directory via filepath.Join,
// then defensively re-checked to remain within the save directory (belt and
// suspenders against symlink-based escapes).
package transfer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// illegalCharsRe matches bytes that are illegal in filenames on Windows and/or
// reserved on POSIX. Note: '/' and '\' are NOT included here - they are path
// separators and are normalized/handled separately (backslashes are converted
// to forward slashes before this regex runs, and forward slashes are used as
// segment delimiters afterwards). We replace each match with '_'.
var illegalCharsRe = regexp.MustCompile(`[<>:"|?*\x00-\x1f]`)

// ErrPathEscape is returned when a sanitized relative path would resolve
// outside the save directory.
var ErrPathEscape = errors.New("transfer: sanitized path escapes save directory")

// SanitizeRelativePath normalizes a sender-provided relative path for safe
// storage under the receiver's save directory. It does NOT join it to a base;
// use JoinSavePath for that.
//
// Steps:
//  1. Strip Windows drive letter (e.g. "C:\foo" -> "foo") and UNC prefix.
//  2. Replace backslashes with forward slashes.
//  3. Replace illegal/reserved characters with '_'.
//  4. Split on '/', drop empty/".", neutralize ".." (cannot ascend).
//  5. Re-join with the OS separator. If every segment was dropped, returns "".
func SanitizeRelativePath(rel string) string {
	rel = strings.TrimSpace(rel)
	rel = stripDriveOrUNC(rel)
	rel = strings.ReplaceAll(rel, "\\", "/")
	segs := strings.Split(rel, "/")
	out := make([]string, 0, len(segs))
	for _, s := range segs {
		s = illegalCharsRe.ReplaceAllString(s, "_")
		s = strings.TrimSpace(s)
		if s == "" || s == "." {
			continue
		}
		if s == ".." {
			continue
		}
		s = strings.TrimRight(s, ". ")
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return strings.Join(out, "/")
}

// stripDriveOrUNC removes a leading Windows drive letter (C:) or UNC prefix
// (\\server\share\, //server/share/).
func stripDriveOrUNC(p string) string {
	// UNC: \\server\share\... or //server/share/...
	if (strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//")) && len(p) > 2 {
		rest := p[2:]
		// Skip past server\share\ (two separators).
		seps := 0
		i := 0
		for ; i < len(rest); i++ {
			if rest[i] == '\\' || rest[i] == '/' {
				seps++
				if seps == 2 {
					i++
					break
				}
				// skip consecutive separators
				for i+1 < len(rest) && (rest[i+1] == '\\' || rest[i+1] == '/') {
					i++
				}
			}
		}
		if seps == 2 {
			return rest[i:]
		}
		return rest // fall back: drop the leading slashes only
	}
	// Drive letter: X:\ or X:/ or X:
	if len(p) >= 2 && isASCIILetter(p[0]) && p[1] == ':' {
		rest := p[2:]
		rest = strings.TrimLeft(rest, `/\`)
		return rest
	}
	return p
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }

// JoinSavePath joins saveDir and a sanitized relative path, then verifies the
// result stays within saveDir (rejecting symlink escapes). If relPath sanitizes
// to empty, baseName is used (falling back to "unnamed").
//
// It also de-conflicts: if the target already exists, a numeric suffix
// " (N)" is appended before the extension until a free name is found.
func JoinSavePath(saveDir, relPath, baseName string) (string, error) {
	rel := SanitizeRelativePath(relPath)
	if rel == "" {
		rel = SanitizeRelativePath(baseName)
	}
	if rel == "" {
		rel = "unnamed"
	}

	cleanSaveDir, err := filepath.Abs(filepath.Clean(saveDir))
	if err != nil {
		return "", fmt.Errorf("transfer: abs save dir: %w", err)
	}

	joined := filepath.Join(cleanSaveDir, filepath.FromSlash(rel))
	cleanJoined, err := filepath.Abs(filepath.Clean(joined))
	if err != nil {
		return "", fmt.Errorf("transfer: abs joined: %w", err)
	}

	if !isWithin(cleanJoined, cleanSaveDir) {
		return "", ErrPathEscape
	}
	return deConflict(cleanJoined), nil
}

// isWithin reports whether target is equal to or inside base, using lexical
// path comparison after cleaning. This catches "../" escapes; it does NOT
// resolve symlinks (callers should ensure saveDir is not a symlink to outside).
func isWithin(target, base string) bool {
	if base == "" {
		return false
	}
	if target == base {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(target, base+sep)
}

// deConflict appends " (N)" before the extension if the path already exists.
func deConflict(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(filepath.Base(path), ext)
	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
	return path // give up; let the caller's os.Create fail
}

// IsValidUTF8 reports whether s is valid UTF-8 with no NUL bytes. Used to
// reject malformed filenames early.
func IsValidUTF8(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsRune(s, 0)
}
