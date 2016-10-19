package ftp

import (
	"errors"
	"net/textproto"
	"strings"
)

var errEmptyCmd = errors.New("got empty command")

// A Command read from or written to a control channel.
type Command struct {
	Cmd string // Cmd is the command type.
	Msg string // Msg is the full message.
}

// Encode c into w.
func (c *Command) Encode(w *textproto.Writer) error {
	if c.Msg == "" {
		return w.PrintfLine("%s", c.Cmd)
	}
	return w.PrintfLine("%s %s", c.Cmd, c.Msg)
}

// Decode from r into c.
func (c *Command) Decode(r *textproto.Reader) error {
	line, err := r.ReadLine()
	if err != nil {
		return err
	}
	s := strings.SplitN(line, " ", 2)
	if s[0] == "" {
		return errEmptyCmd
	}
	c.Cmd = strings.ToUpper(s[0])
	if len(s) > 1 {
		c.Msg = s[1]
	}
	return nil
}

// Args returns the arguments split into tokens.
func (c *Command) Args() []string {
	if c.Msg == "" {
		return nil
	}
	return strings.Split(c.Msg, " ")
}

// A Reply read from or written to a control channel.
type Reply struct {
	Code int
	Msg  string
}

// Encode r into w.
func (r *Reply) Encode(w *textproto.Writer) error {
	lines := r.Lines()
	last := len(lines) - 1
	if len(lines) > 1 {
		if err := w.PrintfLine("%03d-%s", r.Code, lines[0]); err != nil {
			return err
		}
		for _, line := range lines[1:last] {
			if err := w.PrintfLine(" %s", line); err != nil {
				return err
			}
		}
	}
	return w.PrintfLine("%03d %s", r.Code, lines[last])
}

// Decode from tr into r.
func (r *Reply) Decode(tr *textproto.Reader) error {
	code, msg, err := tr.ReadResponse(0)
	if err != nil {
		return err
	}
	r.Code = code
	r.Msg = msg
	return nil
}

// Lines returns r.Msg split by line.
func (r *Reply) Lines() []string {
	msg := strings.Replace(r.Msg, "\r\n", "\n", -1)
	return strings.Split(msg, "\n")
}

// Preliminary returns whether r.Code is 1xx.
func (r *Reply) Preliminary() bool {
	return r.Code >= 100 && r.Code < 200
}

// Success returns whether r.Code is 2xx.
func (r *Reply) Success() bool {
	return r.Code >= 200 && r.Code < 300
}

// Intermediate returns whether r.Code is 3xx.
func (r *Reply) Intermediate() bool {
	return r.Code >= 300 && r.Code < 400
}
