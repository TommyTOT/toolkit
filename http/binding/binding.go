package binding

import (
	"net/http"
)

type Binding interface {
	Name() string
	Bind(request *http.Request, pointer any) error
}

type Body interface {
	Binding
	BindBody(body []byte, pointer any) error
}

type URI interface {
	Name() string
	BindURI(URI map[string][]string, pointer any) error
}

var (
	Form = form{}
)

func Default(method, contentType string) Binding {
	if method == http.MethodGet {
		return Form
	}

	switch contentType {
	//case MIME.JSON:
	//	return JSON
	//case MIME.XML, MIME.XML2:
	//	return XML
	//case MIME.PROTOBUF:
	//	return ProtoBuf
	//case MIME.MSGPACK, MIME.MSGPACK2:
	//	return MsgPack
	//case MIME.YAML:
	//	return YAML
	//case MIME.TOML:
	//	return TOML
	//case MIME.MultipartPOSTForm:
	//	return FormMultipart
	default:
		return Form
	}
}
