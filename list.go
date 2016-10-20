package ftp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"
)

// A Lister produces listing output similar to ls.
type Lister struct {
	File
	Cmd string
	buf *bytes.Buffer
}

// Read implements io.Reader.
func (l *Lister) Read(b []byte) (n int, err error) {
	if l.buf == nil {
		l.buf = new(bytes.Buffer)
		l.WriteTo(l.buf)
	}
	return l.buf.Read(b)
}

// WriteTo implements io.WriterTo.
func (l *Lister) WriteTo(w io.Writer) (n int64, err error) {
	if l.buf != nil {
		return l.buf.WriteTo(w)
	}
	list, err := l.Readdir(0)
	if err != nil {
		return 0, err
	}

	if l.Cmd != "NLST" {
		nn, err := fmt.Fprintln(w, "total", len(list))
		n += int64(nn)
		if err != nil {
			return n, err
		}
	}

	for _, fi := range list {
		nn, err := l.writeLine(w, fi)
		n += int64(nn)
		if err != nil {
			return n, err
		}
	}

	return n, nil
}

func (l *Lister) writeLine(w io.Writer, fi os.FileInfo) (n int, err error) {
	if l.Cmd == "NLST" {
		return fmt.Fprintln(w, fi.Name())
	}
	return fmt.Fprintln(w, listLine(fi))
}

func listLines(fi []os.FileInfo) []string {
	l := make([]string, len(fi))
	for i, fi := range fi {
		l[i] = listLine(fi)
	}
	return l
}

func listLine(fi os.FileInfo) string {
	mode := fi.Mode()
	nlinks := 1
	user := "user"
	group := "group"
	size := fi.Size()
	time := formatTime(fi.ModTime())
	name := fi.Name()

	return fmt.Sprintf("%10s %d %6s %6s %7d %12s %s",
		mode, nlinks, user, group, size, time, name)
}

func formatTime(t time.Time) string {
	if t.Year() == time.Now().Year() {
		return t.Format("Jan _2 15:04")
	}
	return t.Format("Jan _2 2006")
}

type stat struct {
	name string
	size int64
	mode os.FileMode
	time time.Time
}

func (s *stat) Name() string       { return s.name }
func (s *stat) Size() int64        { return s.size }
func (s *stat) ModTime() time.Time { return s.time }
func (s *stat) Mode() os.FileMode  { return s.mode }
func (s *stat) IsDir() bool        { return s.mode.IsDir() }
func (s *stat) Sys() interface{}   { return nil }
