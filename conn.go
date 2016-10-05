package ftp

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand"
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
	return c.addr
}

// Port returns the port associated with c.Addr().
func (c *Conn) Port() (int, error) {
	addr := c.Addr()
	if t, ok := addr.(*net.TCPAddr); ok {
		return t.Port, nil
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, err
	}
	return net.LookupPort(addr.Network(), port)
}

// HostPort is shorthand for HostPort(c.Addr()).
func (c *Conn) HostPort() (string, error) {
	return HostPort(c.Addr())
}

// EHostPort is shorthand for EHostPort("", c.Addr()).
func (c *Conn) EHostPort() (string, error) {
	return EHostPort("", c.Addr())
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

// Attempt to derive an IP and port from addr.
func findIPAndPort(addr net.Addr) (net.IP, int, error) {
	if t, ok := addr.(*net.TCPAddr); ok {
		return t.IP, t.Port, nil
	}
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, 0, err
	}
	p, err := net.LookupPort(addr.Network(), port)
	if err != nil {
		return nil, 0, err
	}
	if host == "" {
		return net.IPv4zero, p, nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, 0, err
	}
	if len(ips) == 0 {
		return net.IPv4zero, p, nil
	}
	i := rand.Intn(len(ips))
	return ips[i], p, nil
}

// HostPort converts addr into a string suitable for use with PASV or PORT.
func HostPort(addr net.Addr) (string, error) {
	h, p, err := findIPAndPort(addr)
	if err != nil {
		return "", err
	}
	if h = h.To4(); h == nil {
		return "", errors.New("unsupported address")
	}
	return fmt.Sprintf("%d,%d,%d,%d,%d,%d",
		h[0], h[1], h[2], h[3], p/256, p%256), nil
}

// EHostPort converts addr into a string suitable for use with EPRT. d is the
// delimiter used in formatting the address, and must either be "" (in which
// case "|" is used) or a single ASCII character in the inclusive range between
// "!" and "~".
func EHostPort(d string, addr net.Addr) (string, error) {
	if d == "" {
		d = "|"
	} else if !validDelimiter(d) {
		return "", errors.New("invalid delimiter")
	}
	h, p, err := findIPAndPort(addr)
	if err != nil {
		return "", err
	}
	var parts []string
	if len(h) == 4 {
		parts = []string{"1", h.String(), strconv.Itoa(p)}
	} else if len(h) == 16 {
		parts = []string{"2", h.String(), strconv.Itoa(p)}
	} else {
		return "", errors.New("unsupported address")
	}
	for _, s := range parts {
		if strings.Contains(s, d) {
			return "", errors.New("invalid delimiter")
		}
	}
	return d + strings.Join(parts, d) + d, nil
}

// ParsePASV extracts an address from a PASV reply message.
func ParsePASV(msg string) (*net.TCPAddr, error) {
	return ParsePORT(deparen(msg))
}

// ParsePORT extracts an address from a PORT command message.
func ParsePORT(msg string) (*net.TCPAddr, error) {
	split := strings.Split(msg, ",")
	if len(split) != 6 {
		return nil, ErrInvalidSyntax
	}
	b := make([]byte, len(split))
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

// ParseEPSV extracts a port from an EPSV reply message.
func ParseEPSV(msg string) (int, error) {
	nw, host, port, err := splitEAddr(deparen(msg))
	if err != nil {
		return 0, err
	}
	if nw != "" || host != "" {
		return 0, ErrInvalidSyntax
	}
	return strconv.Atoi(port)
}

// ParseEPRT extracts an address from an EPRT command message.
func ParseEPRT(msg string) (*net.TCPAddr, error) {
	nw, host, port, err := splitEAddr(msg)
	if err != nil {
		return nil, err
	}
	if nw != "1" && nw != "2" {
		return nil, ErrInvalidSyntax
	}
	p, err := net.LookupPort("tcp", port)
	if err != nil {
		return nil, err
	}
	var ip net.IP
	if nw == "1" {
		ip = net.ParseIP(host).To4()
	} else {
		ip = net.ParseIP(host).To16()
	}
	if ip == nil {
		return nil, ErrInvalidSyntax
	}
	return &net.TCPAddr{IP: ip, Port: p}, nil
}

func splitEAddr(s string) (net, host, port string, err error) {
	if len(s) < 2 {
		return "", "", "", ErrInvalidSyntax
	}
	d := s[0:1]
	if !validDelimiter(d) || !strings.HasSuffix(s, d) {
		return "", "", "", ErrInvalidSyntax
	}
	s = s[1 : len(s)-1]
	split := strings.Split(s, d)
	if len(split) != 3 {
		return "", "", "", ErrInvalidSyntax
	}
	return split[0], split[1], split[2], nil
}

func validDelimiter(d string) bool {
	return len(d) == 1 && d[0] >= '!' && d[0] <= '~'
}

// Extract the contents of the outermost pair of parentheses.
func deparen(s string) string {
	a := strings.Index(s, "(")
	z := strings.LastIndex(s, ")")
	if a == -1 || z == -1 || a >= z {
		return ""
	}
	return s[a+1 : z]
}
