package router

import "net/url"

// Value 值
type Value[Handler any] struct {
	Slash      bool
	Origin     string
	Parameters *Parameters
	Handler    *Handler
}

// Append 添加
func Append[Handler any](start int, path string, end int, count int16, node *Node[Handler], parameters *Parameters, value Value[Handler], unescape bool) {
	if parameters == nil {
		return
	}
	if cap(*parameters) < int(count) {
		list := make(Parameters, len(*parameters), count)
		copy(list, *parameters)
		*parameters = list
	}
	if value.Parameters == nil {
		value.Parameters = parameters
	}
	length := len(*value.Parameters)
	*value.Parameters = (*value.Parameters)[:length+1]
	if end == -1 {
		end = len(path)
	}
	text := path[:end]
	if unescape {
		if item, err := url.QueryUnescape(text); err == nil {
			text = item
		}
	}
	(*value.Parameters)[length] = Parameter{
		Key:   node.path[start:],
		Value: text,
	}
}
