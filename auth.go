package ftp

// An Authorizer can be used with an AuthHandler to authorize login.
type Authorizer interface {
	// Authorize the user. Returning an error closes the session.
	Authorize(user, pass string) (bool, error)
}

// A MapAuthorizer authorizes users from a static map of user names to
// passwords.
type MapAuthorizer map[string]string

// Authorize implements Authorizer.
func (a MapAuthorizer) Authorize(user, pass string) (bool, error) {
	expect, ok := a[user]
	if !ok {
		return false, nil
	}
	return pass == expect, nil
}

// An AuthFunc is an authorizer that calls itself.
type AuthFunc func(user, pass string) (bool, error)

// Authorize implements Authorizer.
func (f AuthFunc) Authorize(user, pass string) (bool, error) {
	return f(user, pass)
}

// AuthAny is an authorizer that accepts any username and password.
var AuthAny = AuthFunc(func(string, string) (bool, error) {
	return true, nil
})

// AuthAnonOnly is an authorizer that only accepts anonymous logins.
var AuthAnonOnly = AuthFunc(func(user, _ string) (bool, error) {
	return user == "anonymous", nil
})

var _ Handler = (*AuthHandler)(nil)

// HandleAuth authorizes a login with the provided Authorizer. If a == nil,
// this performs anonymous authorization.
func HandleAuth(s *Session, a Authorizer) error {
	ah := AuthHandler{a}
	return ah.Handle(s)
}

// An AuthHandler handles login with an Authorizer.
type AuthHandler struct {
	Authorizer // Authorizer to use. Anonymous-only if nil.
}

// Handle implements Handler. This will authorize a user and save the user name
// and password into s.Context.
func (h *AuthHandler) Handle(s *Session) error {
	for {
		c, err := s.Command()
		if err != nil {
			return err
		}
		if err := h.handle(s, c); err != nil {
			return err
		}
		if (c.Cmd == "PASS" && s.User != "") || c.Cmd == "QUIT" {
			return nil
		}
	}
}

// Handle a single command. Login is complete if and only if the last command
// was a PASS and s.User was not reset to "".
func (h *AuthHandler) handle(s *Session, c *Command) error {
	switch c.Cmd {
	case "USER":
		if c.Msg == "" {
			return s.Reply(504, "A user name is required.")
		}
		if h.Authorizer == nil && c.Msg != "anonymous" {
			return s.Reply(331, "This server is anonymous only.")
		}
		s.User = c.Msg
		return s.Reply(331, "Please specify the password.")
	case "PASS":
		if s.User == "" {
			return s.Reply(503, "Log in with USER first.")
		}
		ok, err := h.authorize(s.User, c.Msg)
		if err == nil && ok {
			s.Password = c.Msg
			return s.Reply(230, "Login successful.")
		}
		s.User = ""
		if err != nil {
			return err
		}
		return s.Reply(430, "Invalid user name or password.")
	case "QUIT":
		return s.Reply(211, "Goodbye.")
	default:
		return s.Reply(530, "Log in with USER and PASS.")
	}
}

func (h *AuthHandler) authorize(user, pass string) (bool, error) {
	if h.Authorizer == nil {
		return user == "anonymous", nil
	}
	return h.Authorizer.Authorize(user, pass)
}
