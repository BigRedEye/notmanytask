package targz

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

type Visitor interface {
	VisitDirectory(info fs.FileInfo) error
	VisitFile(info fs.FileInfo) (io.WriteCloser, error)
}

type archiveFileInfo struct {
	fs.FileInfo
	name string
}

func (i archiveFileInfo) Name() string {
	return i.name
}

func cleanArchivePath(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, '\x00') {
		return "", fmt.Errorf("invalid empty archive path")
	}
	// Tar paths use forward slashes. Reject backslashes so that an archive
	// cannot become unsafe when extracted on Windows.
	if strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("invalid archive path %q: backslashes are not allowed", name)
	}
	cleaned := pathpkg.Clean(name)
	if pathpkg.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("archive path %q escapes the destination", name)
	}
	return cleaned, nil
}

// Extract reads a gzip-compressed tar stream and sends safe relative paths to
// visitor. Symbolic links, hard links, devices, and other special entries are
// rejected rather than materialized.
func Extract(input io.Reader, visitor Visitor) (resultErr error) {
	if input == nil {
		return fmt.Errorf("input is nil")
	}
	if visitor == nil {
		return fmt.Errorf("visitor is nil")
	}

	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer func() {
		if err := gzipReader.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close gzip stream: %w", err))
		}
	}()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			if _, err := io.Copy(io.Discard, gzipReader); err != nil {
				return fmt.Errorf("finish gzip stream: %w", err)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar header: %w", err)
		}

		name, err := cleanArchivePath(header.Name)
		if err != nil {
			return err
		}
		info := archiveFileInfo{FileInfo: header.FileInfo(), name: name}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := visitor.VisitDirectory(info); err != nil {
				return fmt.Errorf("create directory %q: %w", name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			writer, err := visitor.VisitFile(info)
			if err != nil {
				return fmt.Errorf("create file %q: %w", name, err)
			}
			if writer == nil {
				return fmt.Errorf("create file %q: visitor returned a nil writer", name)
			}

			_, copyErr := io.Copy(writer, tarReader)
			closeErr := writer.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(
					wrapFileError("write", name, copyErr),
					wrapFileError("close", name, closeErr),
				)
			}
		default:
			return fmt.Errorf("unsupported tar entry %q (type %d)", name, header.Typeflag)
		}
	}
}

func wrapFileError(operation, name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s file %q: %w", operation, name, err)
}

type fsVisitor struct {
	root string
}

func newFSVisitor(root string) (*fsVisitor, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create extraction root: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve extraction root: %w", err)
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return nil, fmt.Errorf("make extraction root absolute: %w", err)
	}
	return &fsVisitor{root: resolvedRoot}, nil
}

func (v *fsVisitor) resolve(name string) (fullPath, relativePath string, err error) {
	relativePath = filepath.FromSlash(name)
	fullPath = filepath.Join(v.root, relativePath)
	rel, err := filepath.Rel(v.root, fullPath)
	if err != nil {
		return "", "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path %q escapes extraction root", name)
	}
	return fullPath, rel, nil
}

func (v *fsVisitor) rejectSymlinkComponents(relativePath string) error {
	if relativePath == "." {
		return nil
	}
	current := v.root
	for _, part := range strings.Split(relativePath, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component %q is a symbolic link", current)
		}
	}
	return nil
}

func (v *fsVisitor) VisitDirectory(info fs.FileInfo) error {
	fullPath, relativePath, err := v.resolve(info.Name())
	if err != nil {
		return err
	}
	if relativePath == "." {
		return nil
	}
	if err := v.rejectSymlinkComponents(relativePath); err != nil {
		return err
	}
	if err := os.MkdirAll(fullPath, info.Mode().Perm()); err != nil {
		return err
	}
	if err := v.rejectSymlinkComponents(relativePath); err != nil {
		return err
	}
	return os.Chmod(fullPath, info.Mode().Perm())
}

func (v *fsVisitor) VisitFile(info fs.FileInfo) (io.WriteCloser, error) {
	fullPath, relativePath, err := v.resolve(info.Name())
	if err != nil {
		return nil, err
	}
	if relativePath == "." {
		return nil, fmt.Errorf("file path resolves to extraction root")
	}
	if err := v.rejectSymlinkComponents(relativePath); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, err
	}
	if err := v.rejectSymlinkComponents(relativePath); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func ExtractToDir(input io.Reader, destination string) error {
	visitor, err := newFSVisitor(destination)
	if err != nil {
		return err
	}
	return Extract(input, visitor)
}
