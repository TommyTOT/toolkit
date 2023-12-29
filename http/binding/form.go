package binding

import "net/http"

type form struct{}

func (form) Name() string {
	return "form"
}

func (form) Bind(request *http.Request, pointer any) error {
	return nil
}
