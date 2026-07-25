// Package authentication holds request credential types.
package authentication

// BaseAuth carries HTTP Basic Auth credentials.
type BaseAuth struct {
	Username string
	Password string
}

// NewBaseAuth builds credentials from a username and password.
func NewBaseAuth(username, password string) BaseAuth {
	return BaseAuth{
		Username: username,
		Password: password,
	}
}

// NewBaseAuthFromMap builds credentials from the --auth flag's key/value map.
func NewBaseAuthFromMap(auth map[string]string) BaseAuth {
	if auth == nil {
		return BaseAuth{}
	}
	return BaseAuth{
		Username: auth["username"],
		Password: auth["password"],
	}
}
