package ftp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"os"
	"path"
	"strconv"
)

var errTransferFailed = errors.New("transfer failed")
var errClosed = errors.New("closed")

// Client for interacting with a server.
type Client struct {
	Addr     string   // Addr of server to connect to.
	Dialer   Dialer   // Dialer for outgoing connections.
	Listener Listener // Listener for incoming connections.
	Debug    bool     // Debug prints control channel traffic.

	*context
}

type context struct {
	laddr net.TCPAddr
	raddr net.TCPAddr
	conn  *textproto.Conn
	ctx   Context
	cwd   string
}

// DialFTP dials a server.
func DialFTP(addr string) (*Client, error) {
	c := &Client{Addr: addr}
	err := c.Connect()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Connect to a server.
func (c *Client) Connect() error {
	if c.Addr == "" {
		return errors.New("no addr to dial")
	}
	if c.context != nil {
		c.Close()
	}
	conn, err := c.dial(c.Addr)
	if err != nil {
		return err
	}
	c.context = &context{
		laddr: *conn.LocalAddr().(*net.TCPAddr),
		raddr: *conn.RemoteAddr().(*net.TCPAddr),
		conn:  textproto.NewConn(conn),
	}
	if r, err := c.reply(); err != nil {
		return err
	} else if !r.Success() {
		return errors.New("unwelcome")
	}
	return nil
}

func (c *Client) connect() error {
	if c.context != nil {
		return nil
	}
	return c.Connect()
}

func (c *Client) dial(addr string) (net.Conn, error) {
	if d := c.Dialer; d != nil {
		return d.Dial("tcp", addr)
	}
	return net.Dial("tcp", addr)
}

func (c *Client) listen(addr string) (net.Listener, error) {
	if l := c.Listener; l != nil {
		return l.Listen("tcp", addr)
	}
	return net.Listen("tcp", addr)
}

// Command sends a command to the server.
func (c *Client) command(cmd, msg string) error {
	if err := c.connect(); err != nil {
		return err
	}
	m := Command{Cmd: cmd, Msg: msg}
	if err := m.Encode(&c.conn.Writer); err != nil {
		return err
	}
	if err := c.conn.W.Flush(); err != nil {
		return err
	}
	if c.Debug {
		fmt.Println(">", m)
	}
	return nil
}

// Reply reads a reply from the server.
func (c *Client) reply() (*Reply, error) {
	if err := c.connect(); err != nil {
		return nil, err
	}
	r := new(Reply)
	if err := r.Decode(&c.conn.Reader); err != nil {
		return nil, err
	}
	if c.Debug {
		fmt.Println("<", r)
	}
	return r, nil
}

// Exchange sends a command and returns the reply.
func (c *Client) exchange(cmd, msg string) (*Reply, error) {
	if err := c.command(cmd, msg); err != nil {
		return nil, err
	}
	for {
		if r, err := c.reply(); err != nil {
			return nil, err
		} else if !r.Preliminary() {
			return r, nil
		}
	}
}

// Authorize with the server.
func (c *Client) Authorize(user, pass string) (bool, error) {
	r, err := c.exchange("USER", user)
	if err != nil {
		return false, err
	}
	if r.Intermediate() {
		if r, err = c.exchange("PASS", pass); err != nil {
			return false, err
		}
	}
	return r.Success(), nil
}

// Data establishes a data channel, preferring to use passive mode, but falling
// back to active mode if that fails.
func (c *Client) data() (*Conn, error) {
	conn, err := c.passive()
	if err != nil {
		return c.active()
	}
	return conn, nil
}

// Passive establishes a data channel by putting the server into passive mode.
func (c *Client) passive() (*Conn, error) {
	addr := c.raddr

	if r, err := c.exchange("EPSV", ""); err != nil {
		return nil, err
	} else if r.Code == 229 {
		p, err := ParseEPSV(r.Msg)
		if err != nil {
			return nil, err
		}
		addr.Port = p
	} else if r, err = c.exchange("PASV", ""); err != nil {
		return nil, err
	} else if r.Success() {
		p, err := ParsePASV(r.Msg)
		if err != nil {
			return nil, err
		}
		addr.Port = p.Port
	} else {
		return nil, errors.New("cannot connect")
	}

	conn, err := c.dial(addr.String())
	if err != nil {
		return nil, err
	}
	return ActiveConn(conn), nil
}

// Active establishes a data channel by putting the server into active mode.
func (c *Client) active() (*Conn, error) {
	addr := c.laddr

	addr.Port = 0

	li, err := c.listen(addr.String())
	if err != nil {
		return nil, err
	}

	conn := PassiveConn(li)

	if r, err := c.exchange("EPRT", conn.EHostPort()); err != nil {
		return nil, err
	} else if !r.Success() {
		return conn, nil
	} else if r, err := c.exchange("PORT", conn.HostPort()); err != nil {
		return nil, err
	} else if !r.Success() {
		return conn, nil
	}

	return nil, errors.New("cannot connect")
}

// Chdir changes to the given directory.
func (c *Client) Chdir(dir string) error {
	r, err := c.exchange("CWD", dir)
	if err != nil {
		return err
	}
	if !r.Success() {
		return errors.New("failed to change directory")
	}

	c.cwd = c.path(dir)
	return nil
}

// Relativize p against the current directory.
func (c *Client) path(p string) string {
	if path.IsAbs(p) {
		return p
	}
	return path.Join(c.cwd, p)
}

// Open a file or directory.
func (c *Client) Open(path string) (File, error) {
	f := &clientFile{
		c:    c,
		path: c.path(path),
	}
	return f, nil
}

// Create a new file.
func (c *Client) Create(path string) (File, error) {
	f := &clientFile{
		c:    c,
		path: c.path(path),
	}
	return f, nil
}

// Mkdir makes a new directory.
func (c *Client) Mkdir(path string) error {
	r, err := c.exchange("MKD", path)
	if err != nil {
		return err
	}
	if !r.Success() {
		return errors.New("failed to make directory")
	}
	return nil
}

// Remove a file or directory.
func (c *Client) Remove(path string) error {
	return nil
}

// Rename a file or directory.
func (c *Client) Rename(old, new string) error {
	return nil
}

// Stat a file or directory.
func (c *Client) Stat(p string) (os.FileInfo, error) {
	mode := os.FileMode(0644)
	if p == "/" {
		mode |= os.ModeDir
	}
	return &stat{
		name: path.Base(p),
		mode: mode,
	}, nil
}

// Close the connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return errors.New("not connected")
	}
	err := c.conn.Close()
	c.context = nil
	return err
}

type clientFile struct {
	c      *Client
	path   string
	conn   *Conn
	seek   int64
	closed bool
	prelim bool
}

// Seek implements File.
func (f *clientFile) Seek(offset int64, whence int) (int64, error) {
	if whence != io.SeekStart || f.conn != nil {
		return 0, errors.New("cannot seek")
	}
	f.seek = offset
	return offset, nil
}

// Read implements File.
func (f *clientFile) Read(b []byte) (n int, err error) {
	if err := f.start("RETR", true); err != nil {
		return 0, err
	}
	if n, err := f.conn.Read(b); err != io.EOF {
		return n, err
	}
	if err := f.finish(); err != nil {
		return n, err
	}
	return n, io.EOF
}

// Write implements File.
func (f *clientFile) Write(b []byte) (n int, err error) {
	if err := f.start("STOR", true); err != nil {
		return 0, err
	}
	return f.conn.Write(b)
}

// Readdir implements File.
func (f *clientFile) Readdir(n int) (fi []os.FileInfo, err error) {
	if err := f.start("NLST", false); err != nil {
		return nil, err
	}
	fi, rerr := f.readdir(n)
	if err := f.finish(); err != nil {
		return fi, err
	}
	return fi, rerr
}

func (f *clientFile) start(cmd string, seek bool) error {
	if f.closed {
		return errClosed
	}
	if f.conn != nil {
		return nil
	}
	conn, err := f.c.data()
	if err != nil {
		return err
	}
	if seek && f.seek != 0 {
		r, err := f.c.exchange("REST", strconv.FormatInt(f.seek, 10))
		if err != nil {
			conn.Close()
			return err
		}
		if !r.Success() {
			conn.Close()
			return errors.New("could not seek")
		}
	}
	f.conn = conn
	return f.c.command(cmd, f.path)
}

func (f *clientFile) finish() error {
	if err := f.conn.Close(); err != nil {
		return err
	}
	f.conn = nil
	for {
		if r, err := f.c.reply(); err != nil {
			return err
		} else if r.Success() {
			return nil
		} else if !r.Preliminary() {
			return errors.New("transfer failed")
		}
	}
}

func (f *clientFile) readdir(n int) (fi []os.FileInfo, err error) {
	for {
		line, err := f.conn.ReadLine()
		if err != nil && err != io.EOF {
			return fi, nil
		}
		if line != "" {
			fi = append(fi, &stat{name: line})
		}
		if err == io.EOF {
			return fi, nil
		}
	}
}

func (f *clientFile) Close() error {
	if f.closed {
		return errors.New("already closed")
	}
	f.closed = true
	if f.conn == nil {
		return nil
	}
	return f.finish()
}
