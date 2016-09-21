package ftp

import "path"

// Context shared between clients and servers.
type Context struct {
	User     string // User name used to authorize the session.
	Password string // Password used to authorize the session.
	Dir      string // Dir is the working directory.
	Mode     string // Mode of data transfer.
	Type     string // Type of data channel.
	Data     *Conn  // Data channel connection.

	EPSVOnly bool // EPSVOnly is true if "EPSV ALL" is set.
}

// Path returns the absolute path of p, using the working directory as the
// base.
func (c *Context) Path(p string) string {
	if path.IsAbs(p) {
		return p
	}
	return path.Join("/", c.Dir, p)
}

// CloseData closes the data connection and sets it to nil. If there is no data
// connection, this returns an error.
func (c *Context) CloseData() error {
	if c.Data == nil {
		return errNoDataConn
	}
	conn := c.Data
	c.Data = nil
	return conn.Close()
}
