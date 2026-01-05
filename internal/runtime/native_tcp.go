package runtime

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// TCP 连接池管理
// ============================================================================

type tcpConnection struct {
	conn   net.Conn
	reader *bufio.Reader
}

var (
	tcpConnections = make(map[int64]*tcpConnection)
	tcpConnMutex   sync.RWMutex
	nextConnID     int64 = 1
)

// ============================================================================
// Native TCP 函数实现 (仅供标准库使用)
// ============================================================================

func nativeTcpConnect(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	host := args[0].AsString()
	port := args[1].AsInt()
	address := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	tcpConnMutex.Lock()
	connID := nextConnID
	nextConnID++
	tcpConnections[connID] = &tcpConnection{conn: conn, reader: bufio.NewReader(conn)}
	tcpConnMutex.Unlock()
	return bytecode.NewInt(connID)
}

func nativeTcpWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	connID := args[0].AsInt()
	data := args[1].AsString()
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.NewInt(-1)
	}
	n, err := tc.conn.Write([]byte(data))
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(n))
}

func nativeTcpRead(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	length := int(args[1].AsInt())
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.NewString("")
	}
	buf := make([]byte, length)
	n, err := tc.reader.Read(buf)
	if err != nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(string(buf[:n]))
}

func nativeTcpReadLine(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.NewString("")
	}
	line, err := tc.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(line)
}

func nativeTcpClose(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	tcpConnMutex.Lock()
	tc, ok := tcpConnections[connID]
	if ok {
		tc.conn.Close()
		delete(tcpConnections, connID)
	}
	tcpConnMutex.Unlock()
	return bytecode.NewBool(ok)
}

func nativeTcpSetTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	seconds := args[1].AsInt()
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	if !ok {
		return bytecode.FalseValue
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	err := tc.conn.SetDeadline(deadline)
	return bytecode.NewBool(err == nil)
}

