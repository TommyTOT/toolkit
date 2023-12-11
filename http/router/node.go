package router

import (
	"bytes"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

var (
	// Colon 冒号
	Colon = []byte(":")
	// Star 星号
	Star = []byte("*")
	// Slash 斜杠
	Slash = []byte("/")
)

// String 字节切片转字符串（没有额外内存分配）
func String(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}

// Bytes 字符串转字节切片（没有额外内存分配）
func Bytes(text string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{text, len(text)},
	))
}

// SBS 将数组中的字节向左移动n个字节（shift bytes）
func SBS(data [4]byte, n int) [4]byte {
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

// LCP 最长公共前缀（longest common prefix）
func LCP(current, compare string) int {
	index := 0
	length := min(len(current), len(compare))
	for index < length && current[index] == compare[index] {
		index++
	}
	return index
}

// Count 字符串按照分割符统计长度
func Count(text string, separators ...[]byte) uint16 {
	var count uint16
	data := Bytes(text)
	for index := range separators {
		count += uint16(bytes.Count(data, separators[index]))
	}
	return count
}

// CPS 统计路径参数（count parameters）
func CPS(path string) uint16 {
	return Count(path, Colon, Star)
}

// CSS 统计路径分段（count sections）
func CSS(path string) uint16 {
	return Count(path, Slash)
}

// FW 搜索通配符段并检查名称中是否有无效字符（find wildcard）
func FW(path string) (string, int, bool) {
	for start, head := range []byte(path) {
		if head != ':' && head != '*' {
			continue
		}
		valid := true
		for end, tail := range []byte(path[start+1:]) {
			switch tail {
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

// Parameter 参数
type Parameter struct {
	Key   string
	Value string
}

// Parameters 参数（复数）
type Parameters []Parameter

// Get 查询
func (p Parameters) Get(name string) (string, bool) {
	for _, entry := range p {
		if entry.Key == name {
			return entry.Value, true
		}
	}
	return "", false
}

// ByName 通过名字查询
func (p Parameters) ByName(name string) (value string) {
	value, _ = p.Get(name)
	return
}

// kind 种类
type kind uint8

const (
	// static 静态
	static kind = iota
	// root 根
	root
	// parameter 参数
	parameter
	// wildcard 通配符
	wildcard
)

// Node 结点
type Node[Handler any] struct {
	// kind 种类
	kind kind
	// wildcard 通配符
	wildcard bool
	// priority 优先级
	priority uint32
	// index 索引
	index string
	// path 路径
	path string
	// origin 原始
	origin string
	// handler 处理器
	handler *Handler
	// children 子结点
	children []*Node[Handler]
}

// add 添加子结点（保持通配结点在最末位）
func (n *Node[Handler]) add(child *Node[Handler]) {
	if n.wildcard && len(n.children) > 0 {
		last := n.children[len(n.children)-1]
		n.children = append(n.children[:len(n.children)-1], child, last)
	} else {
		n.children = append(n.children, child)
	}
}

// increment 增加对应位置子结点优先级（必要时重新排序）
func (n *Node[Handler]) increment(position int) int {
	children := n.children
	children[position].priority++
	priority := children[position].priority
	location := position
	for ; location > 0 && children[location-1].priority < priority; location-- {
		children[location-1], children[location] = children[location], children[location-1]
	}
	if location != position {
		n.index = n.index[:location] + n.index[position:position+1] + n.index[location:position] + n.index[position+1:]
	}
	return location
}

// insert 插入子结点
func (n *Node[Handler]) insert(path string, origin string, handler *Handler) {
	for {
		segment, index, valid := FW(path)
		if index < 0 {
			break
		}
		if !valid {
			panic("only one wildcard per path segment is allowed, has: '" + segment + "' in path '" + origin + "'")
		}
		if len(segment) < 2 {
			panic("wildcards must be named with a non-empty name in path '" + origin + "'")
		}
		if segment[0] == ':' {
			if index > 0 {
				n.path = path[:index]
				path = path[index:]
			}
			child := &Node[Handler]{
				kind:   parameter,
				path:   segment,
				origin: origin,
			}
			n.add(child)
			n.wildcard = true
			n = child
			n.priority++
			if len(segment) < len(path) {
				path = path[len(segment):]
				sub := &Node[Handler]{
					priority: 1,
					origin:   origin,
				}
				n.add(sub)
				n = sub
				continue
			}
			n.handler = handler
			return
		}
		if index+len(segment) != len(path) {
			panic("wildcard routes are only allowed at the end of the path in path '" + origin + "'")
		}
		if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
			prefix := strings.SplitN(n.children[0].path, "/", 2)[0]
			panic("wildcard '" + path + "' in new path '" + origin + "' conflicts with existing path segment '" + prefix + "' in existing prefix '" + n.path + prefix + "'")
		}
		index--
		if path[index] != '/' {
			panic("no / before wildcard in path '" + origin + "'")
		}
		n.path = path[:index]
		child := &Node[Handler]{
			kind:     wildcard,
			wildcard: true,
			origin:   origin,
		}
		n.add(child)
		n.index = string('/')
		n = child
		n.priority++
		child = &Node[Handler]{
			kind:     wildcard,
			priority: 1,
			path:     path[index:],
			origin:   origin,
			handler:  handler,
		}
		n.children = []*Node[Handler]{child}
		return
	}
	n.path = path
	n.origin = origin
	n.handler = handler
}

// search 路径查询（不区分大小写）
func (n *Node[Handler]) search(path string, buffer []byte, runes [4]byte, slash bool) []byte {
	length := len(n.path)
walk:
	for len(path) >= length && (length == 0 || strings.EqualFold(path[1:length], n.path[1:])) {
		origin := path
		path = path[length:]
		buffer = append(buffer, n.path...)
		if len(path) == 0 {
			if n.handler != nil {
				return buffer
			}
			if slash {
				for i, c := range []byte(n.index) {
					if c == '/' {
						n = n.children[i]
						if (len(n.path) == 1 && n.handler != nil) || (n.kind == wildcard && n.children[0].handler != nil) {
							return append(buffer, '/')
						}
						return nil
					}
				}
			}
			return nil
		}
		if !n.wildcard {
			runes = SBS(runes, length)
			if runes[0] != 0 {
				compare := runes[0]
				for index, item := range []byte(n.index) {
					if item == compare {
						n = n.children[index]
						length = len(n.path)
						continue walk
					}
				}
			} else {
				var value rune
				var off int
				for end := min(length, 3); off < end; off++ {
					if index := length - off; utf8.RuneStart(origin[index]) {
						value, _ = utf8.DecodeRuneInString(origin[index:])
						break
					}
				}
				lower := unicode.ToLower(value)
				utf8.EncodeRune(runes[:], lower)
				runes = SBS(runes, off)
				compare := runes[0]
				for index, item := range []byte(n.index) {
					if item == compare {
						if out := n.children[index].search(path, buffer, runes, slash); out != nil {
							return out
						}
						break
					}
				}
				if upper := unicode.ToUpper(value); upper != lower {
					utf8.EncodeRune(runes[:], upper)
					runes = SBS(runes, off)
					compare = runes[0]
					for index, item := range []byte(n.index) {
						if item == compare {
							n = n.children[index]
							length = len(n.path)
							continue walk
						}
					}
				}
			}
			if slash && path == "/" && n.handler != nil {
				return buffer
			}
			return nil
		}
		n = n.children[0]
		switch n.kind {
		case parameter:
			end := 0
			for end < len(path) && path[end] != '/' {
				end++
			}
			buffer = append(buffer, path[:end]...)
			if end < len(path) {
				if len(n.children) > 0 {
					n = n.children[0]
					length = len(n.path)
					path = path[end:]
					continue
				}
				if slash && len(path) == end+1 {
					return buffer
				}
				return nil
			}
			if n.handler != nil {
				return buffer
			}
			if slash && len(n.children) == 1 {
				n = n.children[0]
				if n.path == "/" && n.handler != nil {
					return append(buffer, '/')
				}
			}
			return nil
		case wildcard:
			return append(buffer, path...)
		default:
			panic("invalid node type")
		}
	}
	if slash {
		if path == "/" {
			return buffer
		}
		if len(path)+1 == length && n.path[len(path)] == '/' && strings.EqualFold(path[1:], n.path[1:len(path)]) && n.handler != nil {
			return append(buffer, n.path...)
		}
	}
	return nil
}

// Register 跟据路径注册结点（并发不安全）
func (n *Node[Handler]) Register(path string, handler *Handler) {
	origin := path
	n.priority++
	if len(n.path) == 0 && len(n.children) == 0 {
		n.insert(path, origin, handler)
		n.kind = root
		return
	}
	parent := 0
walk:
	for {
		index := LCP(path, n.path)
		if index < len(n.path) {
			child := Node[Handler]{
				kind:     static,
				wildcard: n.wildcard,
				priority: n.priority - 1,
				index:    n.index,
				path:     n.path[index:],
				origin:   n.origin,
				handler:  n.handler,
				children: n.children,
			}
			n.wildcard = false
			n.index = String([]byte{n.path[index]})
			n.path = path[:index]
			n.origin = origin[:parent+index]
			n.handler = nil
			n.children = []*Node[Handler]{&child}
		}
		if index < len(path) {
			path = path[index:]
			symbol := path[0]
			if n.kind == parameter && symbol == '/' && len(n.children) == 1 {
				parent += len(n.path)
				n = n.children[0]
				n.priority++
				continue walk
			}
			for number, length := 0, len(n.index); number < length; number++ {
				if symbol == n.index[number] {
					parent += len(n.path)
					number = n.increment(number)
					n = n.children[number]
					continue walk
				}
			}
			if symbol != ':' && symbol != '*' && n.kind != wildcard {
				n.index += String([]byte{symbol})
				child := &Node[Handler]{
					origin: origin,
				}
				n.add(child)
				n.increment(len(n.index) - 1)
				n = child
			} else if n.wildcard {
				n = n.children[len(n.children)-1]
				n.priority++
				if len(path) >= len(n.path) && n.path == path[:len(n.path)] && n.kind != wildcard && (len(n.path) >= len(path) || path[len(n.path)] == '/') {
					continue walk
				}
				segment := path
				if n.kind != wildcard {
					segment = strings.SplitN(segment, "/", 2)[0]
				}
				prefix := origin[:strings.Index(origin, segment)] + n.path
				panic("'" + segment + "' in new path '" + origin + "' conflicts with existing wildcard '" + n.path + "' in existing prefix '" + prefix + "'")
			}
			n.insert(path, origin, handler)
			return
		}
		if n.handler != nil {
			panic("handler are already registered for path '" + origin + "'")
		}
		n.origin = origin
		n.handler = handler
		return
	}
}

type Skipped[Handler any] struct {
	Path  string
	Count int16
	Node  *Node[Handler]
}

type Value[Handler any] struct {
	Slash      bool
	Origin     string
	Parameters *Parameters
	Handler    *Handler
}

// Query 根据路径查找结点
func (n *Node[Handler]) Query(path string, parameters *Parameters, skipped *[]Skipped[Handler], unescape bool) (value Value[Handler]) {
	var count int16
walk:
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]
				compare := path[0]
				for index, item := range []byte(n.index) {
					if item == compare {
						if n.wildcard {
							length := len(*skipped)
							*skipped = (*skipped)[:length+1]
							(*skipped)[length] = Skipped[Handler]{
								Path:  prefix + path,
								Count: count,
								Node: &Node[Handler]{
									wildcard: n.wildcard,
									path:     n.path,
									origin:   n.origin,
									kind:     n.kind,
									priority: n.priority,
									handler:  n.handler,
									children: n.children,
								},
							}
						}
						n = n.children[index]
						continue walk
					}
				}
				if !n.wildcard {
					if path != "/" {
						var ok bool
						ok, path, count, n = Find(path, count, n, value, skipped)
						if ok {
							continue walk
						}
						//for length := len(*skipped); length > 0; length-- {
						//	current := (*skipped)[length-1]
						//	*skipped = (*skipped)[:length-1]
						//	if strings.HasSuffix(current.Path, path) {
						//		path = current.Path
						//		n = current.Node
						//		if value.Parameters != nil {
						//			*value.Parameters = (*value.Parameters)[:current.Count]
						//		}
						//		count = current.Count
						//		continue walk
						//	}
						//}
					}
					value.Slash = path == "/" && n.handler != nil
					return
				}
				n = n.children[len(n.children)-1]
				count++
				switch n.kind {
				case parameter:
					end := 0
					for end < len(path) && path[end] != '/' {
						end++
					}
					Append(1, path, end, count, n, parameters, value, unescape)
					//if parameters != nil {
					//	if cap(*parameters) < int(count) {
					//		list := make(Parameters, len(*parameters), count)
					//		copy(list, *parameters)
					//		*parameters = list
					//	}
					//	if value.Parameters == nil {
					//		value.Parameters = parameters
					//	}
					//	length := len(*value.Parameters)
					//	*value.Parameters = (*value.Parameters)[:length+1]
					//	text := path[:end]
					//	if unescape {
					//		if item, err := url.QueryUnescape(text); err == nil {
					//			text = item
					//		}
					//	}
					//	(*value.Parameters)[length] = Parameter{
					//		Key:   n.path[1:],
					//		Value: text,
					//	}
					//}
					if end < len(path) {
						if len(n.children) > 0 {
							path = path[end:]
							n = n.children[0]
							continue walk
						}
						value.Slash = len(path) == end+1
						return
					}
					if value.Handler = n.handler; value.Handler != nil {
						value.Origin = n.origin
						return
					}
					if len(n.children) == 1 {
						n = n.children[0]
						value.Slash = (n.path == "/" && n.handler != nil) || (n.path == "" && n.index == "/")
					}
					return
				case wildcard:
					Append(2, path, -1, count, n, parameters, value, unescape)
					//if parameters != nil {
					//	if cap(*parameters) < int(count) {
					//		list := make(Parameters, len(*parameters), count)
					//		copy(list, *parameters)
					//		*parameters = list
					//	}
					//	if value.Parameters == nil {
					//		value.Parameters = parameters
					//	}
					//	length := len(*value.Parameters)
					//	*value.Parameters = (*value.Parameters)[:length+1]
					//	text := path
					//	if unescape {
					//		if item, err := url.QueryUnescape(path); err == nil {
					//			text = item
					//		}
					//	}
					//	(*value.Parameters)[length] = Parameter{
					//		Key:   n.path[2:],
					//		Value: text,
					//	}
					//}
					value.Handler = n.handler
					value.Origin = n.origin
					return

				default:
					panic("invalid node type")
				}
			}
		}
		if path == prefix {
			if n.handler == nil && path != "/" {
				var ok bool
				ok, path, count, n = Find(path, count, n, value, skipped)
				if ok {
					continue walk
				}
				//for length := len(*skipped); length > 0; length-- {
				//	current := (*skipped)[length-1]
				//	*skipped = (*skipped)[:length-1]
				//	if strings.HasSuffix(current.Path, path) {
				//		path = current.Path
				//		n = current.Node
				//		if value.Parameters != nil {
				//			*value.Parameters = (*value.Parameters)[:current.Count]
				//		}
				//		count = current.Count
				//		continue walk
				//	}
				//}
			}
			if value.Handler = n.handler; value.Handler != nil {
				value.Origin = n.origin
				return
			}
			if path == "/" && n.wildcard && n.kind != root {
				value.Slash = true
				return
			}
			if path == "/" && n.kind == static {
				value.Slash = true
				return
			}
			for index, item := range []byte(n.index) {
				if item == '/' {
					n = n.children[index]
					value.Slash = (len(n.path) == 1 && n.handler != nil) || (n.kind == wildcard && n.children[0].handler != nil)
					return
				}
			}
			return
		}
		value.Slash = path == "/" || (len(prefix) == len(path)+1 && prefix[len(path)] == '/' && path == prefix[:len(prefix)-1] && n.handler != nil)
		if !value.Slash && path != "/" {
			var ok bool
			ok, path, count, n = Find(path, count, n, value, skipped)
			if ok {
				continue walk
			}
			//for length := len(*skipped); length > 0; length-- {
			//	current := (*skipped)[length-1]
			//	*skipped = (*skipped)[:length-1]
			//	if strings.HasSuffix(current.Path, path) {
			//		path = current.Path
			//		n = current.Node
			//		if value.Parameters != nil {
			//			*value.Parameters = (*value.Parameters)[:current.Count]
			//		}
			//		count = current.Count
			//		continue walk
			//	}
			//}
		}
		return
	}
}

func Find[Handler any](path string, count int16, node *Node[Handler], value Value[Handler], skipped *[]Skipped[Handler]) (bool, string, int16, *Node[Handler]) {
	for length := len(*skipped); length > 0; length-- {
		current := (*skipped)[length-1]
		*skipped = (*skipped)[:length-1]
		if strings.HasSuffix(current.Path, path) {
			if value.Parameters != nil {
				*value.Parameters = (*value.Parameters)[:current.Count]
			}
			return true, current.Path, current.Count, current.Node
		}
	}
	return false, path, count, node
}

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

// Search 路径查询（不区分大小写）
func (n *Node[Handler]) Search(path string, slash bool) ([]byte, bool) {
	const size = 128
	buffer := make([]byte, 0, size)
	if length := len(path) + 1; length > size {
		buffer = make([]byte, 0, length)
	}
	data := n.search(path, buffer, [4]byte{}, slash)
	return data, data != nil
}

// Tree 树
type Tree[handler any] struct {
	// method 方法
	method string
	// root 根结点
	root *Node[handler]
}

// Trees 树林
type Trees[handler any] []*Tree[handler]

// Get 获取结点
func (t Trees[handler]) Get(method string) *Node[handler] {
	for index := range t {
		if t[index].method == method {
			return t[index].root
		}
	}
	return nil
}

// NewNode 创建结点
func NewNode[Handler any]() *Node[Handler] {
	return &Node[Handler]{
		origin: "/",
	}
}

// NewTree 创建树
func NewTree[Handler any](method string) *Tree[Handler] {
	return &Tree[Handler]{
		method: method,
		root:   NewNode[Handler](),
	}
}

// NewTrees 创建树林
func NewTrees[Handler any](method ...string) Trees[Handler] {
	list := make(Trees[Handler], 0, len(method))
	for index := range method {
		list = append(list, NewTree[Handler](method[index]))
	}
	return list
}
