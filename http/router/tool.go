package router

import (
	"bytes"
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
