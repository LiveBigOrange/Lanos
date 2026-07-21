package share

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ZipCallback is called for each file added to the ZIP archive.
type ZipCallback func(path string, size int64)

// StreamZip writes a ZIP archive of the given path to w, streaming via
// io.Pipe. For directories, it recursively adds all files. The onFile
// callback is invoked for each file added (useful for progress/logging).
//
// UTF-8 filename handling: each zip.FileHeader has the Language encoding
// flag (0x0800) set so that non-ASCII filenames decode correctly on
// Windows, macOS, and Linux.
func StreamZip(w io.Writer, rootPath string, onFile ZipCallback) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	info, err := os.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("share: stat %s: %w", rootPath, err)
	}

	if !info.IsDir() {
		// Single file
		return addFileToZip(zw, rootPath, info.Name(), onFile)
	}

	// Directory: walk recursively
	base := filepath.Dir(rootPath)
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Relative path from the parent of the shared directory
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return fmt.Errorf("share: rel path %s: %w", path, err)
		}
		// Use forward slashes in ZIP (cross-platform)
		rel = filepath.ToSlash(rel)
		return addFileToZip(zw, path, rel, onFile)
	})
}

// addFileToZip adds a single file to the ZIP archive with UTF-8 flag.
func addFileToZip(zw *zip.Writer, diskPath, zipPath string, onFile ZipCallback) error {
	info, err := os.Stat(diskPath)
	if err != nil {
		return fmt.Errorf("share: stat %s: %w", diskPath, err)
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return fmt.Errorf("share: zip header %s: %w", diskPath, err)
	}
	header.Name = zipPath
	header.Method = zip.Deflate
	// Force UTF-8 flag for cross-platform filename compatibility
	header.Flags |= 0x0800

	writer, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("share: zip create %s: %w", zipPath, err)
	}

	file, err := os.Open(diskPath)
	if err != nil {
		return fmt.Errorf("share: open %s: %w", diskPath, err)
	}
	defer file.Close()

	size, err := io.Copy(writer, file)
	if err != nil {
		return fmt.Errorf("share: zip write %s: %w", zipPath, err)
	}

	if onFile != nil {
		onFile(zipPath, size)
	}
	return nil
}

// CountFiles counts the total number of files and total size in a path
// (file or directory). Used for share status display.
func CountFiles(rootPath string) (count int, totalSize int64, err error) {
	info, err := os.Stat(rootPath)
	if err != nil {
		return 0, 0, fmt.Errorf("share: stat %s: %w", rootPath, err)
	}
	if !info.IsDir() {
		return 1, info.Size(), nil
	}
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			count++
			totalSize += info.Size()
		}
		return nil
	})
	return count, totalSize, err
}

// SanitizeZipPath ensures a ZIP entry path is safe (no traversal, no
// absolute paths, no drive letters).
func SanitizeZipPath(path string) error {
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return fmt.Errorf("share: unsafe zip path %q", path)
	}
	// Windows drive letter check
	if len(path) >= 2 && path[1] == ':' {
		return fmt.Errorf("share: drive letter in zip path %q", path)
	}
	return nil
}
