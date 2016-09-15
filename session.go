package ftp

import (
	"errors"
	"fmt"
	"net"
	"net/textproto"
)

var errSessionClosed = errors.New("session is closed")

// A Session represents a single control channel session with a client.
type Session struct {
	Addr    net.Addr // Addr of remote host.
	Server  *Server  // Server the session belongs to.
	Context          // Context shared with the client.

	conn    *textproto.Conn
	cmd     *Command
	greeted bool
}

// Command reads the next command, or returns the current command if it has
// already been read and has not been replied to. If the greeting has not been
// sent, this will send the greeting first.
func (s *Session) Command() (*Command, error) {
	if s.conn == nil {
		return nil, errSessionClosed
	}
	if !s.greeted {
		if err := s.Reply(220, DefaultGreeting); err != nil {
			return nil, err
		}
	}
	if s.cmd != nil {
		return s.cmd, nil
	}
	s.cmd = new(Command)
	if err := s.cmd.Decode(&s.conn.Reader); err != nil {
		return nil, err
	}
	if s.Server.Debug {
		fmt.Println("<", s.cmd)
	}
	return s.cmd, nil
}

// Reply sends a reply. This must be called with a non-intermediate reply code
// in order to allow the next command to be read. After replying to a QUIT
// command with a non-intermediate response code, the session is closed.
func (s *Session) Reply(code int, msg string, args ...interface{}) error {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	if s.conn == nil {
		return errSessionClosed
	}
	if s.cmd == nil && s.greeted {
		return errors.New("no command to reply to")
	}
	m := Reply{code, msg}
	if s.Server.Debug {
		fmt.Println(">", m)
	}
	if err := m.Encode(&s.conn.Writer); err != nil {
		return err
	}
	if err := s.conn.W.Flush(); err != nil {
		return err
	}
	if code < 200 {
		return nil
	}
	if s.cmd == nil {
		s.greeted = true
		return nil
	}
	quit := s.cmd.Cmd == "QUIT"
	s.cmd = nil
	if quit {
		return s.Close()
	}
	return nil
}

// Close the session. This will send a default goodbye reply if one has not
// been sent in response to a QUIT.
func (s *Session) Close() error {
	if s.conn == nil {
		return errSessionClosed
	}
	if s.cmd != nil || !s.greeted {
		s.Reply(421, DefaultGoodbye)
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

// Active establishes an active data channel connection through the associated
// server's dialer. This sets s.Data and closes any existing data channel.
func (s *Session) Active(addr net.Addr) error {
	if s.Data != nil {
		s.Data.Close()
		s.Data = nil
	}
	c, err := s.Server.dial(addr.Network(), addr.String())
	if err != nil {
		return err
	}
	s.Data = ActiveConn(c)
	s.Data.Type(s.Type)
	return nil
}

// Passive creates a passive connection listening through the associated
// server's listener. This sets s.Data and closes any existing data channel.
func (s *Session) Passive(nw string) error {
	if s.Data != nil {
		s.Data.Close()
		s.Data = nil
	}
	li, err := s.Server.listen(nw, s.passiveAddr())
	if err != nil {
		return err
	}
	if s.Data != nil {
		s.Data.Close()
	}
	s.Data = PassiveConn(li)
	s.Data.Type(s.Type)
	return nil
}

// Return an addr with a wildcard port and the same host as the control
// channel.
func (s *Session) passiveAddr() string {
	if s.Server.Addr == "" {
		return ":0"
	}
	host, _, err := net.SplitHostPort(s.Server.Addr)
	if err != nil {
		return ":0"
	}
	return net.JoinHostPort(host, "0")
}

// SetType sets s.Type as well as the type of any existing data channel.
func (s *Session) SetType(t string) error {
	switch t {
	case "L8", "I":
		t = "I"
	case "A", "AN":
		t = "A"
	case "AT", "AC":
		return errors.New("ASCII print mode is not supported.")
	case "E", "EN", "ET", "EC":
		return errors.New("EBCDIC mode is not supported.")
	default:
		return errors.New("Unrecognized type.")
	}
	s.Type = t
	if s.Data != nil {
		s.Data.Type(t)
	}
	return nil
}

// SetMode sets s.Mode as well as the mode of any existing data channel.
func (s *Session) SetMode(m string) error {
	switch m {
	case "S":
	case "B":
		return errors.New("Block mode is not supported.")
	case "C":
		return errors.New("Compressed mode is not supported.")
	default:
		return errors.New("Unrecognized mode.")
	}
	s.Mode = m
	if s.Data != nil {
		s.Data.Mode(m)
	}
	return nil
}
