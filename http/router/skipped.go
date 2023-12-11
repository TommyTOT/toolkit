package router

import (
	"strings"
)

// Skipped 跳过
type Skipped[Handler any] struct {
	// Path 路径
	Path string
	// Count 参数数量
	Count int16
	// Node 结点
	Node *Node[Handler]
}

// Find 查询
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
