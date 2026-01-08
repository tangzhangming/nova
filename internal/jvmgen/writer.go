package jvmgen

import (
	"bytes"
	"encoding/binary"
)

// ByteWriter 字节码写入器
type ByteWriter struct {
	buf bytes.Buffer
}

// NewByteWriter 创建新的字节码写入器
func NewByteWriter() *ByteWriter {
	return &ByteWriter{}
}

// WriteByte 写入单个字节
func (w *ByteWriter) WriteByte(b byte) {
	w.buf.WriteByte(b)
}

// WriteU8 写入无符号字节
func (w *ByteWriter) WriteU8(v uint8) {
	w.buf.WriteByte(v)
}

// WriteU16 写入无符号短整型 (大端序)
func (w *ByteWriter) WriteU16(v uint16) {
	binary.Write(&w.buf, binary.BigEndian, v)
}

// WriteU32 写入无符号整型 (大端序)
func (w *ByteWriter) WriteU32(v uint32) {
	binary.Write(&w.buf, binary.BigEndian, v)
}

// WriteBytes 写入字节数组
func (w *ByteWriter) WriteBytes(b []byte) {
	w.buf.Write(b)
}

// Bytes 返回字节数组
func (w *ByteWriter) Bytes() []byte {
	return w.buf.Bytes()
}

// Len 返回当前长度
func (w *ByteWriter) Len() int {
	return w.buf.Len()
}

// Reset 重置写入器
func (w *ByteWriter) Reset() {
	w.buf.Reset()
}
