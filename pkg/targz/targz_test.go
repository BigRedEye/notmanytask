package targz

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type archiveEntry struct {
	name     string
	typeflag byte
	body     string
	mode     int64
	linkname string
}

func makeArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{
			Name:     entry.name,
			Mode:     mode,
			Typeflag: typeflag,
			Linkname: entry.linkname,
		}
		if typeflag == tar.TypeReg || typeflag == tar.TypeRegA {
			header.Size = int64(len(entry.body))
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			if _, err := io.WriteString(tarWriter, entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func TestExtractToDirCreatesNestedTree(t *testing.T) {
	destination := t.TempDir()
	existing := filepath.Join(destination, "nested", "file.txt")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("old"), 0o777); err != nil {
		t.Fatal(err)
	}
	archive := makeArchive(t,
		archiveEntry{name: "nested", typeflag: tar.TypeDir, mode: 0o1750},
		archiveEntry{name: "nested/file.txt", body: "hello", mode: 0o640},
		archiveEntry{name: "implicit/parent/file.txt", body: "world", mode: 0o600},
	)
	if err := ExtractToDir(bytes.NewReader(archive), destination); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"nested/file.txt":          "hello",
		"implicit/parent/file.txt": "world",
	} {
		content, err := os.ReadFile(filepath.Join(destination, path))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Errorf("%s contains %q, want %q", path, content, want)
		}
	}
	fileInfo, err := os.Stat(filepath.Join(destination, "nested/file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o640 {
		t.Errorf("file mode = %o, want 640", fileInfo.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Join(destination, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o750 {
		t.Errorf("directory mode = %o, want 750", dirInfo.Mode().Perm())
	}
	if dirInfo.Mode()&fs.ModeSticky == 0 {
		t.Error("directory sticky bit was not preserved")
	}
}

func TestExtractDefersRestrictiveDirectoryModes(t *testing.T) {
	destination := t.TempDir()
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(destination, "readonly"), 0o700)
		_ = os.Chmod(filepath.Join(destination, "readonly", "nested"), 0o700)
		_ = os.Chmod(filepath.Join(destination, "readonly", "nested", "file.txt"), 0o600)
	})
	archive := makeArchive(t,
		archiveEntry{name: "readonly", typeflag: tar.TypeDir, mode: 0o555},
		archiveEntry{name: "readonly/nested", typeflag: tar.TypeDir, mode: 0o500},
		archiveEntry{name: "readonly/nested/file.txt", body: "created after restrictive directories", mode: 0o400},
	)
	if err := ExtractToDir(bytes.NewReader(archive), destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "readonly", "nested", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "created after restrictive directories" {
		t.Fatalf("unexpected content: %q", content)
	}
	for path, wantMode := range map[string]fs.FileMode{
		"readonly":        0o555,
		"readonly/nested": 0o500,
	} {
		info, err := os.Stat(filepath.Join(destination, path))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != wantMode {
			t.Errorf("%s mode = %o, want %o", path, info.Mode().Perm(), wantMode)
		}
	}
}

func TestExtractRejectsUnsafePaths(t *testing.T) {
	tests := []string{
		"../escape.txt",
		"inside/../../escape.txt",
		"/absolute.txt",
		`windows\escape.txt`,
	}
	for _, name := range tests {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			destination := t.TempDir()
			err := ExtractToDir(bytes.NewReader(makeArchive(t, archiveEntry{name: name, body: "bad"})), destination)
			if err == nil {
				t.Fatalf("unsafe path %q was accepted", name)
			}
		})
	}
}

func TestCleanArchivePathRejectsNULAndPlatformVolumes(t *testing.T) {
	if _, err := cleanArchivePath("bad\x00path"); err == nil || !strings.Contains(err.Error(), "NUL") {
		t.Fatalf("NUL path returned %v", err)
	}

	name := "C:/escape.txt"
	if filepath.VolumeName(filepath.FromSlash(name)) != "" {
		if _, err := cleanArchivePath(name); err == nil {
			t.Fatalf("volume-qualified path %q was accepted", name)
		}
	}
}

func TestExtractRejectsSpecialEntries(t *testing.T) {
	for _, typeflag := range []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
		t.Run(fmt.Sprintf("type_%d", typeflag), func(t *testing.T) {
			err := ExtractToDir(bytes.NewReader(makeArchive(t, archiveEntry{
				name: "special", typeflag: typeflag, linkname: "../outside",
			})), t.TempDir())
			if err == nil || !strings.Contains(err.Error(), "unsupported tar entry") {
				t.Fatalf("entry type %d returned %v", typeflag, err)
			}
		})
	}
}

func TestExtractDoesNotFollowExistingSymlink(t *testing.T) {
	destination := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(destination, "linked")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	archive := makeArchive(t, archiveEntry{name: "linked/escape.txt", body: "bad"})
	err := ExtractToDir(bytes.NewReader(archive), destination)
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive wrote through symlink: %v", err)
	}
}

type recordingVisitor struct {
	directories []string
	files       []string
	writers     []*trackingWriter
}

func (v *recordingVisitor) VisitDirectory(info fs.FileInfo) error {
	v.directories = append(v.directories, info.Name())
	return nil
}

func (v *recordingVisitor) VisitFile(info fs.FileInfo) (io.WriteCloser, error) {
	if len(v.writers) > 0 && !v.writers[len(v.writers)-1].closed {
		return nil, errors.New("previous writer is still open")
	}
	v.files = append(v.files, info.Name())
	w := &trackingWriter{}
	v.writers = append(v.writers, w)
	return w, nil
}

type trackingWriter struct {
	buffer   bytes.Buffer
	writeErr error
	closeErr error
	closed   bool
}

func (w *trackingWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.buffer.Write(data)
}

func (w *trackingWriter) Close() error {
	w.closed = true
	return w.closeErr
}

func TestExtractPassesFullPathsAndClosesFilesPromptly(t *testing.T) {
	visitor := &recordingVisitor{}
	archive := makeArchive(t,
		archiveEntry{name: "nested", typeflag: tar.TypeDir},
		archiveEntry{name: "nested/first.txt", body: "first"},
		archiveEntry{name: "nested/second.txt", body: "second"},
	)
	if err := Extract(bytes.NewReader(archive), visitor); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(visitor.directories, []string{"nested"}) {
		t.Fatalf("directories = %#v", visitor.directories)
	}
	if !reflect.DeepEqual(visitor.files, []string{"nested/first.txt", "nested/second.txt"}) {
		t.Fatalf("files = %#v", visitor.files)
	}
	for _, writer := range visitor.writers {
		if !writer.closed {
			t.Fatal("writer was not closed")
		}
	}
}

type singleWriterVisitor struct {
	writer io.WriteCloser
}

func (v singleWriterVisitor) VisitDirectory(fs.FileInfo) error { return nil }
func (v singleWriterVisitor) VisitFile(fs.FileInfo) (io.WriteCloser, error) {
	return v.writer, nil
}

func TestExtractReportsWriteAndCloseErrors(t *testing.T) {
	writeFailure := errors.New("write failed")
	closeFailure := errors.New("close failed")
	writer := &trackingWriter{writeErr: writeFailure, closeErr: closeFailure}
	err := Extract(bytes.NewReader(makeArchive(t, archiveEntry{name: "file", body: "data"})), singleWriterVisitor{writer: writer})
	if !errors.Is(err, writeFailure) || !errors.Is(err, closeFailure) {
		t.Fatalf("combined error = %v", err)
	}
	if !writer.closed {
		t.Fatal("writer was not closed after write failure")
	}
}

func TestExtractValidatesInputsAndDestination(t *testing.T) {
	visitor := &recordingVisitor{}
	if err := Extract(nil, visitor); err == nil {
		t.Fatal("nil input was accepted")
	}
	if err := Extract(bytes.NewReader(nil), nil); err == nil {
		t.Fatal("nil visitor was accepted")
	}
	if err := Extract(bytes.NewReader([]byte("not gzip")), visitor); err == nil {
		t.Fatal("invalid gzip was accepted")
	}
	corrupted := makeArchive(t, archiveEntry{name: "file", body: "content"})
	corrupted[len(corrupted)-1] ^= 0xff
	if err := Extract(bytes.NewReader(corrupted), visitor); err == nil {
		t.Fatal("invalid gzip checksum was accepted")
	}

	parent := t.TempDir()
	destination := filepath.Join(parent, "file")
	if err := os.WriteFile(destination, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ExtractToDir(bytes.NewReader(makeArchive(t)), destination); err == nil {
		t.Fatal("file destination was accepted as a directory")
	}
}
