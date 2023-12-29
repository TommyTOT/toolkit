package node

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Parameter struct {
	Key   string
	Value string
}

type Parameters []Parameter

func (p Parameters) Get(name string) (string, bool) {
	for _, entry := range p {
		if entry.Key == name {
			return entry.Value, true
		}
	}
	return "", false
}

func (p Parameters) ByName(name string) (value string) {
	value, _ = p.Get(name)
	return
}

var (
	colon = []byte(":")
	star  = []byte("*")
	slash = []byte("/")
)

func CountParameters(path string) uint16 {
	var count uint16
	data := StringToBytes(path)
	count += uint16(bytes.Count(data, colon))
	count += uint16(bytes.Count(data, star))
	return count
}

func CountSections(path string) uint16 {
	data := StringToBytes(path)
	return uint16(bytes.Count(data, slash))
}

func findWildcard(path string) (wildcard string, index int, valid bool) {
	for start, initiate := range []byte(path) {
		if initiate != ':' && initiate != '*' {
			continue
		}
		valid = true
		for end, finish := range []byte(path[start+1:]) {
			switch finish {
			case '/':
				return path[start : start+1+end], start, valid
			case ':', '*':
				valid = false
			}
		}
		return path[start:], start, valid
	}
	return "", -1, false
}

func minimum(current, compare int) int {
	if current <= compare {
		return current
	}
	return compare
}

func longestCommonPrefix(current, compare string) int {
	index := 0
	length := minimum(len(current), len(compare))
	for index < length && current[index] == compare[index] {
		index++
	}
	return index
}

func shiftNRuneBytes(data [4]byte, n int) [4]byte {
	switch n {
	case 0:
		return data
	case 1:
		return [4]byte{data[1], data[2], data[3], 0}
	case 2:
		return [4]byte{data[2], data[3]}
	case 3:
		return [4]byte{data[3]}
	default:
		return [4]byte{}
	}
}

type kind uint8

const (
	static kind = iota
	root
	parameter
	catchall
)

type Node[Value any] struct {
	key      string
	kind     kind
	path     string
	index    string
	value    *Value
	priority uint32
	wildcard bool
	children []*Node[Value]
}

func (n *Node[Value]) addChild(child *Node[Value]) {
	if n.wildcard && len(n.children) > 0 {
		wildcard := n.children[len(n.children)-1]
		n.children = append(n.children[:len(n.children)-1], child, wildcard)
	} else {
		n.children = append(n.children, child)
	}
}

func (n *Node[Value]) incrementChildPriority(position int) int {
	children := n.children
	children[position].priority++
	priority := children[position].priority
	current := position
	for ; current > 0 && children[current-1].priority < priority; current-- {
		children[current-1], children[current] = children[current], children[current-1]
	}
	if current != position {
		n.index = n.index[:current] + n.index[position:position+1] + n.index[current:position] + n.index[position+1:]
	}
	return current
}

func (n *Node[Value]) insertChild(key string, path string, value *Value) error {
	node := n
	for {
		wildcard, index, valid := findWildcard(key)
		if index < 0 {
			break
		}
		if !valid {
			return fmt.Errorf("only one wildcard per path segment is allowed, has: '%s' in path '%s'", wildcard, path)
		}
		if len(wildcard) < 2 {
			return fmt.Errorf("wildcards must be named with a non-empty name in path '%s'", path)
		}
		if wildcard[0] == ':' {
			if index > 0 {
				node.key = key[:index]
				key = key[index:]
			}
			child := &Node[Value]{
				kind: parameter,
				key:  wildcard,
				path: path,
			}
			node.addChild(child)
			node.wildcard = true
			node = child
			node.priority++
			if len(wildcard) < len(key) {
				key = key[len(wildcard):]
				sub := &Node[Value]{
					priority: 1,
					path:     path,
				}
				node.addChild(sub)
				node = child
				continue
			}
			node.value = value
			return nil
		}
		if index+len(wildcard) != len(key) {
			return fmt.Errorf("catch-all routes are only allowed at the end of the path in path '%s'", path)
		}
		if len(node.key) > 0 && node.key[len(node.key)-1] == '/' {
			segment := strings.SplitN(node.children[0].key, "/", 2)[0]
			return fmt.Errorf("catch-all wildcard '%s' in new path '%s' conflicts with existing path segment '%s' in existing prefix '%s%s'", key, path, segment, node.key, segment)
		}
		index--
		if key[index] != '/' {
			return fmt.Errorf("no / before catch-all in path '%s'", path)
		}
		node.key = key[:index]
		child := &Node[Value]{
			wildcard: true,
			kind:     catchall,
			path:     path,
		}
		node.addChild(child)
		node.index = string('/')
		node = child
		node.priority++
		child = &Node[Value]{
			key:      key[index:],
			kind:     catchall,
			value:    value,
			priority: 1,
			path:     path,
		}
		node.children = []*Node[Value]{child}
		return nil
	}
	node.key = key
	node.value = value
	node.path = path
	return nil
}

func (n *Node[Value]) findCaseInsensitivePathRecursive(path string, caseInsensitivePath []byte, runeBytes [4]byte, fixTrailingSlash bool) ([]byte, error) {
	node := n
	length := len(node.key)
walk:
	for len(path) >= length && (length == 0 || strings.EqualFold(path[1:length], node.key[1:])) {
		origin := path
		path = path[length:]
		caseInsensitivePath = append(caseInsensitivePath, node.key...)
		if len(path) == 0 {
			if node.value != nil {
				return caseInsensitivePath, nil
			}
			if fixTrailingSlash {
				for index, symbol := range []byte(node.index) {
					if symbol == '/' {
						node = node.children[index]
						if (len(node.key) == 1 && node.value != nil) || (node.kind == catchall && node.children[0].value != nil) {
							return append(caseInsensitivePath, '/'), nil
						}
						return nil, nil
					}
				}
			}
			return nil, nil
		}
		if !node.wildcard {
			runeBytes = shiftNRuneBytes(runeBytes, length)
			if runeBytes[0] != 0 {
				character := runeBytes[0]
				for index, symbol := range []byte(node.index) {
					if symbol == character {
						node = node.children[index]
						length = len(node.key)
						continue walk
					}
				}
			} else {
				var value rune
				var off int
				for scope := minimum(length, 3); off < scope; off++ {
					if index := length - off; utf8.RuneStart(origin[index]) {
						value, _ = utf8.DecodeRuneInString(origin[index:])
						break
					}
				}
				lower := unicode.ToLower(value)
				utf8.EncodeRune(runeBytes[:], lower)
				runeBytes = shiftNRuneBytes(runeBytes, off)
				character := runeBytes[0]
				for index, symbol := range []byte(node.index) {
					if symbol == character {
						if out, exception := node.children[index].findCaseInsensitivePathRecursive(path, caseInsensitivePath, runeBytes, fixTrailingSlash); exception != nil {
							return nil, exception
						} else if out != nil {
							return out, nil
						}
						break
					}
				}
				if upper := unicode.ToUpper(value); upper != lower {
					utf8.EncodeRune(runeBytes[:], upper)
					runeBytes = shiftNRuneBytes(runeBytes, off)
					mark := runeBytes[0]
					for index, symbol := range []byte(node.index) {
						if symbol == mark {
							node = node.children[index]
							length = len(node.path)
							continue walk
						}
					}
				}
			}
			if fixTrailingSlash && path == "/" && node.value != nil {
				return caseInsensitivePath, nil
			}
			return nil, nil
		}
		node = node.children[0]
		switch node.kind {
		case parameter:
			end := 0
			for end < len(path) && path[end] != '/' {
				end++
			}
			caseInsensitivePath = append(caseInsensitivePath, path[:end]...)
			if end < len(path) {
				if len(node.children) > 0 {
					node = node.children[0]
					length = len(node.key)
					path = path[end:]
					continue
				}
				if fixTrailingSlash && len(path) == end+1 {
					return caseInsensitivePath, nil
				}
				return nil, nil
			}
			if node.value != nil {
				return caseInsensitivePath, nil
			}
			if fixTrailingSlash && len(node.children) == 1 {
				node = node.children[0]
				if node.key == "/" && node.value != nil {
					return append(caseInsensitivePath, '/'), nil
				}
			}
			return nil, nil
		case catchall:
			return append(caseInsensitivePath, path...), nil
		default:
			return nil, fmt.Errorf("invalid node type")
		}
	}
	if fixTrailingSlash {
		if path == "/" {
			return caseInsensitivePath, nil
		}
		if len(path)+1 == length && node.key[len(path)] == '/' && strings.EqualFold(path[1:], node.key[1:len(path)]) && node.value != nil {
			return append(caseInsensitivePath, node.key...), nil
		}
	}
	return nil, nil
}

func (n *Node[Value]) addRoute(path string, value *Value) error {
	origin := path
	node := n
	node.priority++
	if len(node.key) == 0 && len(node.children) == 0 {
		exception := node.insertChild(path, origin, value)
		if exception != nil {
			return exception
		}
		node.kind = root
		return nil
	}
	parent := 0
walk:
	for {
		index := longestCommonPrefix(path, node.key)
		if index < len(node.key) {
			child := Node[Value]{
				key:      node.key[index:],
				wildcard: node.wildcard,
				kind:     static,
				index:    node.index,
				children: node.children,
				value:    node.value,
				priority: node.priority - 1,
				path:     node.path,
			}
			node.children = []*Node[Value]{&child}
			node.index = BytesToString([]byte{node.path[index]})
			node.key = path[:index]
			node.value = nil
			node.wildcard = false
			node.path = origin[:parent+index]
		}
		if index < len(path) {
			path = path[index:]
			symbol := path[0]
			if node.kind == parameter && symbol == '/' && len(node.children) == 1 {
				parent += len(node.key)
				node = node.children[0]
				node.priority++
				continue walk
			}
			for start, end := 0, len(node.index); start < end; start++ {
				if symbol == node.index[start] {
					parent += len(node.key)
					start = node.incrementChildPriority(start)
					node = node.children[start]
					continue walk
				}
			}
			if symbol != ':' && symbol != '*' && node.kind != catchall {
				node.index += BytesToString([]byte{symbol})
				child := &Node[Value]{
					path: origin,
				}
				node.addChild(child)
				node.incrementChildPriority(len(node.index) - 1)
				node = child
			} else if node.wildcard {
				node = node.children[len(node.children)-1]
				node.priority++
				if len(path) >= len(node.key) && node.key == path[:len(node.key)] && node.kind != catchall && (len(node.key) >= len(path) || path[len(node.key)] == '/') {
					continue walk
				}
				segment := path
				if node.kind != catchall {
					segment = strings.SplitN(segment, "/", 2)[0]
				}
				prefix := origin[:strings.Index(origin, segment)] + node.key
				return fmt.Errorf("'%s' in new path '%s' conflicts with existing wildcard '%s' in existing prefix '%s'", segment, origin, node.key, prefix)
			}
			return node.insertChild(path, origin, value)
		}
		if node.value != nil {
			return fmt.Errorf("value are already registered for path '%s'", origin)
		}
		node.value = value
		node.path = origin
		return nil
	}
}

func (n *Node[Value]) findCaseInsensitivePath(path string, fixTrailingSlash bool) ([]byte, bool, error) {
	const size = 128
	buffer := make([]byte, 0, size)
	if length := len(path) + 1; length > size {
		buffer = make([]byte, 0, length)
	}
	data, exception := n.findCaseInsensitivePathRecursive(path, buffer, [4]byte{}, fixTrailingSlash)
	return data, data != nil, exception
}

type nodeValue[Value any] struct {
	value            *Value
	parameters       *Parameters
	fixTrailingSlash bool
	fullPath         string
}

type skippedNode[Value any] struct {
	path            string
	node            *Node[Value]
	parametersCount int16
}

func (n *Node[Value]) getValue(path string, parameters *Parameters, skippedNodes *[]skippedNode[Value], unescape bool) (value nodeValue[Value], exception error) {
	node := n
	var globalParametersCount int16
walk:
	for {
		prefix := node.key
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]
				character := path[0]
				for index, symbol := range []byte(node.index) {
					if symbol == character {
						if node.wildcard {
							length := len(*skippedNodes)
							*skippedNodes = (*skippedNodes)[:length+1]
							(*skippedNodes)[length] = skippedNode[Value]{
								path: prefix + path,
								node: &Node[Value]{
									key:      node.key,
									wildcard: node.wildcard,
									kind:     node.kind,
									priority: node.priority,
									children: node.children,
									value:    node.value,
									path:     node.path,
								},
								parametersCount: globalParametersCount,
							}
						}
						node = node.children[index]
						continue walk
					}
				}
				if !node.wildcard {
					if path != "/" {
						for length := len(*skippedNodes); length > 0; length-- {
							current := (*skippedNodes)[length-1]
							*skippedNodes = (*skippedNodes)[:length-1]
							if strings.HasSuffix(current.path, path) {
								path = current.path
								node = current.node
								if value.parameters != nil {
									*value.parameters = (*value.parameters)[:current.parametersCount]
								}
								globalParametersCount = current.parametersCount
								continue walk
							}
						}
					}
					value.fixTrailingSlash = path == "/" && node.value != nil
					return
				}
				node = node.children[len(node.children)-1]
				globalParametersCount++
				switch node.kind {
				case parameter:
					end := 0
					for end < len(path) && path[end] != '/' {
						end++
					}
					if parameters != nil {
						if cap(*parameters) < int(globalParametersCount) {
							newParameters := make(Parameters, len(*parameters), globalParametersCount)
							copy(newParameters, *parameters)
							*parameters = newParameters
						}
						if value.parameters == nil {
							value.parameters = parameters
						}
						length := len(*value.parameters)
						*value.parameters = (*value.parameters)[:length+1]
						pathValue := path[:end]
						if unescape {
							if realValue, currentException := url.QueryUnescape(pathValue); currentException == nil {
								pathValue = realValue
							}
						}
						(*value.parameters)[length] = Parameter{
							Key:   node.key[1:],
							Value: pathValue,
						}
					}
					if end < len(path) {
						if len(node.children) > 0 {
							path = path[end:]
							node = node.children[0]
							continue walk
						}
						value.fixTrailingSlash = len(path) == end+1
						return
					}
					if value.value = node.value; value.value != nil {
						value.fullPath = node.path
						return
					}
					if len(node.children) == 1 {
						node = node.children[0]
						value.fixTrailingSlash = (node.key == "/" && node.value != nil) || (node.key == "" && node.index == "/")
					}
					return
				case catchall:
					if parameters != nil {
						if cap(*parameters) < int(globalParametersCount) {
							newParameters := make(Parameters, len(*parameters), globalParametersCount)
							copy(newParameters, *parameters)
							*parameters = newParameters
						}
						if value.parameters == nil {
							value.parameters = parameters
						}
						length := len(*value.parameters)
						*value.parameters = (*value.parameters)[:length+1]
						pathValue := path
						if unescape {
							if realValue, currentException := url.QueryUnescape(path); currentException == nil {
								pathValue = realValue
							}
						}
						(*value.parameters)[length] = Parameter{
							Key:   node.key[2:],
							Value: pathValue,
						}
					}
					value.value = node.value
					value.fullPath = node.path
					return
				default:
					exception = fmt.Errorf("invalid node type")
					return
				}
			}
		}
		if path == prefix {
			if node.value == nil && path != "/" {
				for length := len(*skippedNodes); length > 0; length-- {
					current := (*skippedNodes)[length-1]
					*skippedNodes = (*skippedNodes)[:length-1]
					if strings.HasSuffix(current.path, path) {
						path = current.path
						node = current.node
						if value.parameters != nil {
							*value.parameters = (*value.parameters)[:current.parametersCount]
						}
						globalParametersCount = current.parametersCount
						continue walk
					}
				}
			}
			if value.value = node.value; value.value != nil {
				value.fullPath = node.path
				return
			}
			if path == "/" && node.wildcard && node.kind != root {
				value.fixTrailingSlash = true
				return
			}
			if path == "/" && node.kind == static {
				value.fixTrailingSlash = true
				return
			}
			for index, symbol := range []byte(node.index) {
				if symbol == '/' {
					node = node.children[index]
					value.fixTrailingSlash = (len(node.key) == 1 && node.value != nil) || (node.kind == catchall && node.children[0].value != nil)
					return
				}
			}
			return
		}
		value.fixTrailingSlash = path == "/" || (len(prefix) == len(path)+1 && prefix[len(path)] == '/' && path == prefix[:len(prefix)-1] && node.value != nil)
		if !value.fixTrailingSlash && path != "/" {
			for length := len(*skippedNodes); length > 0; length-- {
				current := (*skippedNodes)[length-1]
				*skippedNodes = (*skippedNodes)[:length-1]
				if strings.HasSuffix(current.path, path) {
					path = current.path
					node = current.node
					if value.parameters != nil {
						*value.parameters = (*value.parameters)[:current.parametersCount]
					}
					globalParametersCount = current.parametersCount
					continue walk
				}
			}
		}
		return
	}
}

type tree[Value any] struct {
	method string
	root   *Node[Value]
}

type trees[Value any] []tree[Value]

func (t trees[Value]) get(method string) *Node[Value] {
	for _, item := range t {
		if item.method == method {
			return item.root
		}
	}
	return nil
}
