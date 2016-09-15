package ftp

import (
	"net"
	"net/textproto"
)

// DefaultGreeting is the default greeting for new connections.
var DefaultGreeting = "Welcome."

// DefaultGoodbye is the default goodbye message for closing sessions.
var DefaultGoodbye = "Goodbye."

// A Dialer establishes an outgoing connection.
type Dialer interface {
	Dial(net, addr string) (net.Conn, error)
}

// A Listener listens for incoming connections.
type Listener interface {
	Listen(net, addr string) (net.Listener, error)
}

// A Server serves incoming connections.
type Server struct {
	Addr     string   // Addr to bind to.
	Dialer   Dialer   // Dialer for active connections.
	Listener Listener // Listener for passive connections.
	Handler  Handler  // Handler for commands.
	Debug    bool     // Debug prints control channel traffic.
}

// Listen through the server's listener.
func (s *Server) listen(nw, addr string) (net.Listener, error) {
	if s.Listener != nil {
		return s.Listener.Listen(nw, addr)
	}
	return net.Listen(nw, addr)
}

// Dial through the server's dialer.
func (s *Server) dial(nw, addr string) (net.Conn, error) {
	if s.Dialer != nil {
		return s.Dialer.Dial(nw, addr)
	}
	return net.Dial(nw, addr)
}

// ListenAndServe listens on s.Addr and serves incoming connections.
func (s *Server) ListenAndServe() error {
	a := s.Addr
	if a == "" {
		a = ":ftp"
	}
	l, err := s.listen("tcp", a)
	if err != nil {
		return err
	}
	return s.Serve(l)
}

// Serve incoming connections over l.
func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.ServeFTP(c)
	}
}

// ServeFTP serves one client.
func (s *Server) ServeFTP(c net.Conn) {
	tc := textproto.NewConn(c)
	ss := Session{
		Addr:   c.RemoteAddr(),
		Server: s,
		conn:   tc,
	}
	if s.Handler != nil {
		s.Handler.Handle(&ss)
	}
	ss.Close()
}
