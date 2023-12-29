//go:build !go1.20

package node

import (
	"unsafe"
)

func StringToBytes(text string) []byte {
	return *(*[]byte)(unsafe.Pointer(
		&struct {
			string
			Cap int
		}{text, len(text)},
	))
}

func BytesToString(data []byte) string {
	return *(*string)(unsafe.Pointer(&data))
}
