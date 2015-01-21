package hello

import (
	"net/http"
)

func Hello(req *http.Request, msg *string) error {
	*msg = "Hello World"
	return nil
}
