package ftp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrInvalidSyntax is returned by parsing functions if the input could not be
// parsed.
var ErrInvalidSyntax = errors.New("invalid syntax")

// A Conn represents a data channel. This transforms data according to the
// transfer type and also performs buffering.
type Conn struct {
	w    *bufio.Writer
	r    *bufio.Reader
	typ  string
	mode string
	addr net.Addr

	passive net.Listener
	active  net.Conn
	err     error
	m       struct {
		sync.Mutex
		sync.Cond
	}

	cr bool // ASCII mode: whether we've written a CR
}

// ActiveConn creates an active connection over c.
func ActiveConn(c net.Conn) *Conn {
	if c == nil {
		panic("conn may not be nil")
	}
	conn := &Conn{
		addr:   c.RemoteAddr(),
		active: c,
	}
	conn.m.L = &conn.m
	return conn
}

// PassiveConn creates a passive connection over l. This will close l once a
// connection has been established.
func PassiveConn(l net.Listener) *Conn {
	if l == nil {
		panic("listener may not be nil")
	}
	conn := &Conn{
		addr:    l.Addr(),
		passive: l,
	}
	conn.m.L = &conn.m
	go conn.listen()
	return conn
}

// Type sets the transfer type of the connection.
func (c *Conn) Type(t string) {
	c.m.Lock()
	c.typ = t
	c.m.Unlock()
}

// Mode sets the transfer mode of the connection.
func (c *Conn) Mode(m string) {
	c.m.Lock()
	c.mode = m
	c.m.Unlock()
}

func (c *Conn) listen() {
	if c.active != nil {
		panic("active connection already established")
	}
	conn, err := c.passive.Accept()
	c.m.Lock()
	c.active, c.err = conn, err
	c.passive.Close()
	c.m.Unlock()
	c.m.Broadcast()
}

func (c *Conn) accept() (net.Conn, error) {
	c.m.Lock()
	for c.active == nil || c.err == nil {
		c.m.Wait()
	}
	conn, err := c.active, c.err
	c.m.Unlock()
	return conn, err
}

// Passive returns whether this is a passive connection.
func (c *Conn) Passive() bool {
	c.m.Lock()
	b := c.passive != nil
	c.m.Unlock()
	return b
}

// Active returns whether this is an active connection or a passive one that
// has accepted a connection.
func (c *Conn) Active() bool {
	c.m.Lock()
	b := c.active != nil
	c.m.Unlock()
	return b
}

// Addr returns the listening address if this is a passive connection.
// Otherwise, this returns the remote address of the active connection.
func (c *Conn) Addr() net.Addr {
	var addr net.Addr
	c.m.Lock()
	if c.passive != nil {
		addr = c.passive.Addr()
	} else {
		addr = c.active.RemoteAddr()
	}
	c.m.Unlock()
	return addr
}

// HostPort is shorthand for HostPort(c.Addr()).
func (c *Conn) HostPort() (string, error) {
	return HostPort(c.Addr())
}

// Read implements io.Reader. If a connection has not been established, this
// waits for a connection.
func (c *Conn) Read(b []byte) (n int, err error) {
	c.m.Lock()
	for c.active == nil && c.err == nil {
		c.m.Wait()
	}
	if err := c.err; err != nil {
		c.m.Unlock()
		return 0, err
	}
	if c.r == nil {
		c.r = bufio.NewReader(c.active)
	}
	r := c.r
	c.m.Unlock()

	return r.Read(b)
}

// Write implements io.Writer. If a connection has not been established, this
// waits for a connection.
func (c *Conn) Write(b []byte) (n int, err error) {
	c.m.Lock()
	for c.active == nil && c.err == nil {
		c.m.Wait()
	}
	if err := c.err; err != nil {
		c.m.Unlock()
		return 0, err
	}
	if c.w == nil {
		c.w = bufio.NewWriter(c.active)
	}
	w, typ := c.w, c.typ
	c.m.Unlock()

	if typ == "A" {
		return c.writeASCII(w, b)
	}
	return w.Write(b)
}

func (c *Conn) writeASCII(w *bufio.Writer, b []byte) (n int, err error) {
	for _, b := range b {
		if !c.cr && b == '\n' {
			if err = w.WriteByte('\r'); err != nil {
				break
			}
		} else {
			c.cr = b == '\r'
		}
		if err = w.WriteByte(b); err != nil {
			break
		}
		n++
	}
	return
}

// Flush any buffered data.
func (c *Conn) Flush() error {
	c.m.Lock()
	w := c.w
	c.m.Unlock()
	if w != nil && w.Buffered() > 0 {
		return w.Flush()
	}
	return nil
}

// Close flushes and closes the connection.
func (c *Conn) Close() (err error) {
	if err := c.Flush(); err != nil {
	}
	c.m.Lock()
	if c.active != nil {
		err = c.active.Close()
	} else {
		err = c.passive.Close()
	}
	c.m.Unlock()
	return err
}

// LocalAddr waits for a connection, then calls LocalAddr on it.
func (c *Conn) LocalAddr() net.Addr {
	conn, err := c.accept()
	if err != nil {
		return nil
	}
	return conn.LocalAddr()
}

// RemoteAddr waits for a connection, then calls RemoteAddr on it.
func (c *Conn) RemoteAddr() net.Addr {
	conn, err := c.accept()
	if err != nil {
		return nil
	}
	return conn.RemoteAddr()
}

// SetDeadline waits for a connection, then calls SetDeadline on it.
func (c *Conn) SetDeadline(t time.Time) error {
	conn, err := c.accept()
	if err != nil {
		return err
	}
	return conn.SetDeadline(t)
}

// SetReadDeadline waits for a connection, then calls SetReadDeadline on it.
func (c *Conn) SetReadDeadline(t time.Time) error {
	conn, err := c.accept()
	if err != nil {
		return err
	}
	return conn.SetReadDeadline(t)
}

// SetWriteDeadline waits for a connection, then calls SetWriteDeadline on it.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	conn, err := c.accept()
	if err != nil {
		return err
	}
	return conn.SetWriteDeadline(t)
}

func findIPAndPort(addr net.Addr) (net.IP, int, error) {
	switch addr := addr.(type) {
	case *net.TCPAddr:
		return addr.IP, addr.Port, nil
	case *net.UDPAddr:
		return addr.IP, addr.Port, nil
	}
	return nil, 0, fmt.Errorf("unknown addr type: %T", addr)
}

// HostPort converts addr into a string suitable for use with PASV or PORT.
func HostPort(addr net.Addr) (string, error) {
	i, p, err := findIPAndPort(addr)
	if err != nil {
		return "", err
	}
	if len(i) != 4 {
		return "", errors.New("addr is not IPv4")
	}
	return fmt.Sprintf("%d,%d,%d,%d,%d,%d",
		i[0], i[1], i[2], i[3], p/256, p%256), nil
}

// ParseAddr parses an address from a PORT command.
func ParseHostPort(s string) (*net.TCPAddr, error) {
	split := strings.Split(s, ",")
	if len(split) != 6 {
		return nil, errors.New("invalid syntax")
	}
	b := make([]byte, len(s))
	for i, s := range split {
		n, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return nil, err
		}
		b[i] = byte(n)
	}
	return &net.TCPAddr{
		IP:   net.IP(b[:4]),
		Port: int(binary.BigEndian.Uint16(b[4:])),
	}, nil
}
