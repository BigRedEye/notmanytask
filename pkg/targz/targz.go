package targz

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
)

type Visitor interface {
	VisitDirectory(info fs.FileInfo) error
	VisitFile(info fs.FileInfo) (io.WriteCloser, error)
}

func Extract(input io.Reader, visitor Visitor) error {
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		info := header.FileInfo()
		if info.IsDir() {
			err = visitor.VisitDirectory(info)
			if err != nil {
				return err
			}
		} else {
			writer, err := visitor.VisitFile(info)
			if err != nil {
				return err
			}

			defer writer.Close()

			buf := make([]byte, 16384)
			for {
				count, err := tarReader.Read(buf)
				if err == io.EOF {
					break
				}
				if err != nil {
					return err
				}

				written, err := writer.Write(buf[:count])
				if err != nil {
					return err
				}
				if written < count {
					return fmt.Errorf("failed to write %d bytes", count)
				}
			}

			err = writer.Close()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type fsVisitor struct {
	root string
}

func (f *fsVisitor) VisitDirectory(info fs.FileInfo) error {
	return os.MkdirAll(info.Name(), info.Mode())
}

func (v *fsVisitor) VisitFile(info fs.FileInfo) (io.WriteCloser, error) {
	return os.MkdirAll(info.Name(), info.Mode())
}

func ExtractToDir(input io.Reader, path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return nil
	}

	return nil
}
