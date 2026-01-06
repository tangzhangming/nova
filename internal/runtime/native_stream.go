package runtime

import (
	"bufio"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 文件流管理
// ============================================================================

type fileStream struct {
	file   *os.File
	reader *bufio.Reader
	mode   string
}

var (
	fileStreamPool   = make(map[int]*fileStream)
	fileStreamNextID = 1
	fileStreamMutex  sync.Mutex
)

// ============================================================================
// Native 流操作函数
// ============================================================================

// nativeStreamOpen 打开文件流
func (r *Runtime) nativeStreamOpen(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	path := args[0].AsString()
	mode := args[1].AsString()

	var flag int
	switch mode {
	case "r":
		flag = os.O_RDONLY
	case "w":
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case "a":
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case "r+":
		flag = os.O_RDWR
	case "w+":
		flag = os.O_RDWR | os.O_CREATE | os.O_TRUNC
	case "a+":
		flag = os.O_RDWR | os.O_CREATE | os.O_APPEND
	default:
		return bytecode.NewInt(-1)
	}

	f, err := os.OpenFile(path, flag, 0644)
	if err != nil {
		return bytecode.NewInt(-1)
	}

	fileStreamMutex.Lock()
	id := fileStreamNextID
	fileStreamNextID++
	fileStreamPool[id] = &fileStream{
		file:   f,
		reader: bufio.NewReader(f),
		mode:   mode,
	}
	fileStreamMutex.Unlock()

	return bytecode.NewInt(int64(id))
}

// nativeStreamRead 读取指定长度
func (r *Runtime) nativeStreamRead(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	id := int(args[0].AsInt())
	length := int(args[1].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewString("")
	}

	buf := make([]byte, length)
	n, err := stream.file.Read(buf)
	if err != nil && err != io.EOF {
		return bytecode.NewString("")
	}
	return bytecode.NewString(string(buf[:n]))
}

// nativeStreamReadLine 读取一行
func (r *Runtime) nativeStreamReadLine(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewString("")
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewString("")
	}

	line, err := stream.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return bytecode.NewString("")
	}
	return bytecode.NewString(strings.TrimRight(line, "\r\n"))
}

// nativeStreamWrite 写入内容
func (r *Runtime) nativeStreamWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	id := int(args[0].AsInt())
	content := args[1].AsString()

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewInt(-1)
	}

	n, err := stream.file.WriteString(content)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(n))
}

// nativeStreamSeek 移动文件指针
func (r *Runtime) nativeStreamSeek(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.FalseValue
	}
	id := int(args[0].AsInt())
	offset := args[1].AsInt()
	whence := int(args[2].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.FalseValue
	}

	_, err := stream.file.Seek(offset, whence)
	if err != nil {
		return bytecode.FalseValue
	}
	// 重置 reader
	stream.reader.Reset(stream.file)
	return bytecode.TrueValue
}

// nativeStreamTell 获取当前位置
func (r *Runtime) nativeStreamTell(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.NewInt(-1)
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.NewInt(-1)
	}

	pos, err := stream.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(pos)
}

// nativeStreamEof 检查是否到达文件末尾
func (r *Runtime) nativeStreamEof(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.TrueValue
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.TrueValue
	}

	// 尝试读取一个字节来检查是否到达末尾
	currentPos, _ := stream.file.Seek(0, io.SeekCurrent)
	buf := make([]byte, 1)
	_, err := stream.file.Read(buf)
	stream.file.Seek(currentPos, io.SeekStart)
	stream.reader.Reset(stream.file)

	return bytecode.NewBool(err == io.EOF)
}

// nativeStreamFlush 刷新缓冲区
func (r *Runtime) nativeStreamFlush(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	fileStreamMutex.Unlock()
	if !ok || stream.file == nil {
		return bytecode.FalseValue
	}

	err := stream.file.Sync()
	return bytecode.NewBool(err == nil)
}

// nativeStreamClose 关闭文件流
func (r *Runtime) nativeStreamClose(args []bytecode.Value) bytecode.Value {
	if len(args) == 0 {
		return bytecode.FalseValue
	}
	id := int(args[0].AsInt())

	fileStreamMutex.Lock()
	stream, ok := fileStreamPool[id]
	if ok {
		delete(fileStreamPool, id)
	}
	fileStreamMutex.Unlock()

	if !ok || stream.file == nil {
		return bytecode.FalseValue
	}

	err := stream.file.Close()
	return bytecode.NewBool(err == nil)
}


