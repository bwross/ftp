package ftp

import (
	"errors"
	"net"
	"net/textproto"
)

var (
	errNotConnected     = errors.New("not connected")
	errAlreadyConnected = errors.New("already connected")
)

// Client for interacting with a server.
type Client struct {
	Dialer *net.Dialer // Dialer for connections.

	conn *textproto.Conn
}

// Dial dials and connects to addr.
func (c *Client) Dial(addr string) error {
	if c.conn != nil {
		return errAlreadyConnected
	}
	conn, err := c.dial(addr)
	if err != nil {
		return err
	}
	return c.connect(conn)
}

func (c *Client) dial(addr string) (net.Conn, error) {
	if d := c.Dialer; d != nil {
		return d.Dial("tcp", addr)
	}
	return net.Dial("tcp", addr)
}

// Connect the client to conn.
func (c *Client) Connect(conn net.Conn) error {
	if c.conn != nil {
		return errAlreadyConnected
	}
	return c.connect(conn)
}

func (c *Client) connect(conn net.Conn) error {
	c.conn = textproto.NewConn(conn)
	return nil
}

// Get a file.
func (c *Client) Get(path string) *Reader {
	panic("TODO")
}

// Put a file.
func (c *Client) Put(path string) *Writer {
	panic("TODO")
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// Reader for a file.
type Reader struct {
	conn *net.Conn
}

// Writer for a file.
type Writer struct {
	conn *net.Conn
}
