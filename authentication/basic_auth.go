package authentication

// "net/http"
// "fmt"

type BaseAuth struct {
	Username string
	Password string
}

func NewBaseRequest(username, password string) BaseAuth {
	return BaseAuth{
		Username: username,
		Password: password,
	}
}

func NewBaseRequestFromMap(auth map[string]string) BaseAuth {
	if auth == nil {
		return BaseAuth{}
	}
	return BaseAuth{
		Username: auth["username"],
		Password: auth["password"],
	}
}
