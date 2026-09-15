package java

import (
	"bufio"
	"bytes"
	"testing"
)

func FuzzLSPFraming(f *testing.F) {
	f.Add([]byte("Content-Length: 31\r\n\r\n{\"jsonrpc\":\"2.0\",\"method\":\"x\"}"))
	f.Fuzz(func(t *testing.T, frame []byte) {
		_, _ = ReadMessage(bufio.NewReader(bytes.NewReader(frame)), 1<<20)
	})
}
