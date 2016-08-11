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

	mode := fi.Mode()
	nlinks := 1
	user := "user"
	group := "group"
	size := fi.Size()
	time := formatTime(fi.ModTime())
	name := fi.Name()

	return fmt.Fprintln(w, mode, nlinks, user, group, size, time, name)
}

func formatTime(t time.Time) string {
	if t.Year() == time.Now().Year() {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("Jan 2 2006")
}
