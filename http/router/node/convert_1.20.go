//go:build go1.20

package node

import (
	"unsafe"
)

func StringToBytes(text string) []byte {
	return unsafe.Slice(unsafe.StringData(text), len(text))
}

func BytesToString(data []byte) string {
	return unsafe.String(unsafe.SliceData(data), len(data))
}
