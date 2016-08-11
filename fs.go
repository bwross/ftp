package ftp

import (
	"io"
	"os"
	"path"
)

// FileSystem is the interface expected by a FileHandler. This type is intended
// to work with the os package. If errors returned by these methods match
// errors returned by os package functions, more informative reply codes may be
// chosen by a FileHandler in response to failed commands.
type FileSystem interface {
	Create(path string) (File, error)      // Create a new file.
	Open(path string) (File, error)        // Open a file or directory.
	Stat(path string) (os.FileInfo, error) // Stat a file or directory.
	Mkdir(path string) error               // Mkdir makes a new directory.
}

// File is the interface returned by certain FileSystem methods.
type File interface {
	io.Reader
	io.Writer
	io.Closer

	// Readdir has semantics like os.Readdir.
	Readdir(n int) ([]os.FileInfo, error)
}

// LocalFileSystem is a FileSystem implementation that calls os package
// functions.
type LocalFileSystem struct {
	Root string // Root of the file system, or current directory if "".
}

func (f *LocalFileSystem) Create(path string) (File, error) {
	return os.Create(f.path(path))
}

func (f *LocalFileSystem) Open(path string) (File, error) {
	return os.Open(f.path(path))
}

func (f *LocalFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(f.path(path))
}

func (f *LocalFileSystem) Mkdir(path string) error {
	return os.Mkdir(f.path(path), 0755)
}

func (f *LocalFileSystem) path(p string) string {
	p = path.Join("/", p) // Prevent directory traversal.
	if f.Root == "" {
		return path.Join(".", p)
	}
	return path.Join(f.Root, p)
}
