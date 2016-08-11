package ftp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const mdtmFormat = "20060102150405"

var errNoDataConn = errors.New("no data channel connection")

// A Handler for a session.
type Handler interface {
	// Handle a session. It is optional to send a greeting or reply to a QUIT.
	// The session is closed by the Server on return. The return value is for
	// composing Handlers and is ignored by the Server.
	Handle(*Session) error
}

var _ Handler = (*FileHandler)(nil)

// ServeFiles serves files from fs.
func ServeFiles(s *Session, fs FileSystem) error {
	fh := FileHandler{FileSystem: fs}
	return fh.Handle(s)
}

// A FileHandler serves from a FileSystem.
type FileHandler struct {
	Authorizer // Authorizer for login, skipped if nil.
	FileSystem // FileSystem to serve.
}

// Handle implements Handler.
func (h *FileHandler) Handle(s *Session) error {
	if h.Authorizer != nil {
		if err := HandleAuth(s, h.Authorizer); err != nil {
			return err
		}
	}

	for {
		c, err := s.Command()
		if err != nil {
			return err
		}
		if err := h.handle(s, c); err != nil {
			return err
		}
		if c.Cmd == "QUIT" {
			return io.EOF
		}
	}
}

func (h *FileHandler) handle(s *Session, c *Command) error {
	switch c.Cmd {
	case "USER":
		return s.Reply(530, "Cannot change user.")
	case "PASS":
		return s.Reply(230, "Already logged in.")
	case "SYST":
		return s.Reply(215, "UNIX Type: L8")
	case "TYPE":
		if err := s.SetType(c.Msg); err != nil {
			return s.Reply(504, err.Error())
		}
		return s.Reply(200, "Type switched successfully.")
	case "MODE":
		if err := s.SetMode(c.Msg); err != nil {
			return s.Reply(504, err.Error())
		}
		return s.Reply(200, "Mode switched successfully.")
	case "PWD":
		return s.Reply(200, s.Path(""))
	case "CWD":
		if c.Msg == "" {
			return s.Reply(550, "Failed to change directory.")
		}
		path := s.Path(c.Msg)
		if stat, err := h.Stat(path); isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if isNotExist(err) {
			return s.Reply(550, "No such directory.")
		} else if err != nil || !stat.IsDir() {
			return s.Reply(550, "Failed to change directory.")
		}
		s.Dir = path
		return s.Reply(250, "Directory successfully changed.")
	case "CDUP":
		path := s.Path("..")
		if stat, err := h.Stat(path); isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if isNotExist(err) {
			return s.Reply(550, "No such directory.")
		} else if err != nil || !stat.IsDir() {
			return s.Reply(550, "Failed to change directory.")
		}
		s.Dir = path
		return s.Reply(250, "Directory successfully changed.")
	case "MKD":
		path := s.Path(c.Msg)
		if err := h.Mkdir(path); err != nil {
			return s.Reply(550, "Failed to create directory.")
		}
		return s.Reply(257, `"`+c.Msg+`" created.`)
	case "SIZE":
		path := s.Path(c.Msg)
		stat, err := h.Stat(path)
		if isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if isNotExist(err) {
			return s.Reply(550, "No such file or directory.")
		} else if err != nil {
			return s.Reply(550, "Could not get size.")
		} else if stat.IsDir() {
			return s.Reply(550, "Path specifies a directory.")
		}
		size := strconv.FormatInt(stat.Size(), 10)
		return s.Reply(213, size)
	case "MDTM":
		path := s.Path(c.Msg)
		stat, err := h.Stat(path)
		if isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if isNotExist(err) {
			return s.Reply(550, "No such file.")
		} else if err != nil || stat.IsDir() {
			return s.Reply(550, "Could not get size.")
		}
		mdtm := stat.ModTime().Format(mdtmFormat)
		return s.Reply(213, mdtm)
	case "PASV":
		hp, err := s.Passive()
		if err != nil {
			return s.Reply(425, "Can't open data connection.")
		}
		msg := fmt.Sprintf("Entering Passive Mode (%s).", hp)
		return s.Reply(227, msg)
	case "PORT":
		if err := s.Active(c.Msg); err == ErrInvalidSyntax {
			return s.Reply(501, "Invalid syntax.")
		} else if err != nil {
			return s.Reply(550, "Failed to connect.")
		}
		return s.Reply(227, "OK")
	case "LIST", "NLST":
		if err := h.list(s, c); err == errNoDataConn {
			return s.Reply(425, "Use PORT or PASV first.")
		} else if isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if isNotExist(err) {
			return s.Reply(550, "No such directory.")
		} else if err != nil {
			return s.Reply(550, "Error listing directory.")
		}
		return s.Reply(226, "Directory send OK.")
	case "RETR":
		if err := h.retrieve(s, c); err == errNoDataConn {
			return s.Reply(425, "Use PORT or PASV first.")
		} else if isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if isNotExist(err) {
			return s.Reply(550, "No such file.")
		} else if err != nil {
			return s.Reply(550, "Error retrieving file.")
		}
		return s.Reply(226, "Transfer complete.")
	case "STOR":
		if err := h.store(s, c); err == errNoDataConn {
			return s.Reply(425, "Use PORT or PASV first.")
		} else if isPermission(err) {
			return s.Reply(550, "Insufficient permissions.")
		} else if err != nil {
			return s.Reply(550, "Error storing file.")
		}
		return s.Reply(226, "Transfer complete.")
	case "NOOP":
		return s.Reply(200, "OK.")
	case "QUIT":
		return s.Reply(211, "Goodbye.")
	default:
		return s.Reply(502, "Not implemented.")
	}
}

// Handler for RETR.
func (h *FileHandler) retrieve(s *Session, c *Command) error {
	if s.Data == nil {
		return errNoDataConn
	}
	path := s.Path(c.Msg)
	file, err := h.Open(path)
	if err != nil {
		s.Data.Close()
		return err
	}
	if err := s.Reply(150, "Here comes the file."); err != nil {
		file.Close()
		s.Data.Close()
		return err
	}
	if _, err := io.Copy(s.Data, file); err != nil {
		file.Close()
		s.Data.Close()
		return err
	}
	file.Close()
	return s.Data.Close()
}

// Handler for STOR.
func (h *FileHandler) store(s *Session, c *Command) error {
	if s.Data == nil {
		return errNoDataConn
	}
	path := s.Path(c.Msg)
	file, err := h.Create(path)
	if err != nil {
		s.Data.Close()
		return err
	}
	if err := s.Reply(150, "Awaiting file data."); err != nil {
		file.Close()
		s.Data.Close()
		return err
	}
	if _, err := io.Copy(s.Data, file); err != nil {
		file.Close()
		s.Data.Close()
		return err
	}
	err = file.Close()
	s.Data.Close()
	return err
}

// Handler for LIST and NLST.
func (h *FileHandler) list(s *Session, c *Command) error {
	if s.Data == nil {
		return errNoDataConn
	}
	path := s.Path(stripListFlags(c.Msg))
	file, err := h.Open(path)
	if err != nil {
		s.Data.Close()
		return err
	}
	if err := s.Reply(150, "Here comes the list."); err != nil {
		file.Close()
		s.Data.Close()
		return err
	}
	list := Lister{
		File: file,
		Cmd:  c.Cmd,
	}
	if _, err := list.WriteTo(s.Data); err != nil {
		file.Close()
		s.Data.Close()
		return err
	}
	file.Close()
	return s.Data.Close()
}

// Some clients assume LIST accepts flags like ls. This removes those.
func stripListFlags(s string) string {
	for _, c := range s {
		if c == '-' {
			break
		} else if c != ' ' {
			return s
		}
	}
	ss := strings.Split(s, " ")
	out := ss[:0]
	for _, s := range ss {
		if !strings.HasPrefix(s, "-") {
			out = append(out, s)
		}
	}
	return strings.Join(out, " ")
}

// Check if an error is a permission error.
func isPermission(err error) bool {
	return os.IsPermission(err)
}

// Check if an error implies a file does not exist.
func isNotExist(err error) bool {
	return os.IsNotExist(err)
}

// Check if an error implies a file already exists.
func isExist(err error) bool {
	return os.IsPermission(err)
}
