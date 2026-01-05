package runtime

import (
	"bufio"
	"crypto/tls"
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
	conn       net.Conn
	reader     *bufio.Reader
	isTLS      bool
	readBufSize  int
	writeBufSize int
}

var (
	tcpConnections = make(map[int64]*tcpConnection)
	tcpConnMutex   sync.RWMutex
	nextConnID     int64 = 1
)

// getConnection 获取连接（线程安全）
func getConnection(connID int64) (*tcpConnection, bool) {
	tcpConnMutex.RLock()
	tc, ok := tcpConnections[connID]
	tcpConnMutex.RUnlock()
	return tc, ok
}

// registerConnection 注册新连接（线程安全）
func registerConnection(conn net.Conn, isTLS bool) int64 {
	tcpConnMutex.Lock()
	connID := nextConnID
	nextConnID++
	tcpConnections[connID] = &tcpConnection{
		conn:   conn,
		reader: bufio.NewReader(conn),
		isTLS:  isTLS,
	}
	tcpConnMutex.Unlock()
	return connID
}

// ============================================================================
// 连接管理函数
// ============================================================================

// nativeTcpConnect 连接到TCP服务器（默认10秒超时）
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
	connID := registerConnection(conn, false)
	return bytecode.NewInt(connID)
}

// nativeTcpConnectTimeout 带超时的连接（毫秒）
func nativeTcpConnectTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 3 {
		return bytecode.NewInt(-1)
	}
	host := args[0].AsString()
	port := args[1].AsInt()
	timeoutMs := args[2].AsInt()
	
	address := fmt.Sprintf("%s:%d", host, port)
	timeout := time.Duration(timeoutMs) * time.Millisecond
	
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	connID := registerConnection(conn, false)
	return bytecode.NewInt(connID)
}

// nativeTcpClose 关闭连接
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

// nativeTcpIsConnected 检查连接是否有效
func nativeTcpIsConnected(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	
	// 尝试设置一个很短的读取超时来探测连接状态
	tc.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	one := make([]byte, 1)
	_, err := tc.conn.Read(one)
	tc.conn.SetReadDeadline(time.Time{}) // 重置超时
	
	if err != nil {
		// 如果是超时错误，说明连接仍然有效
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return bytecode.TrueValue
		}
		return bytecode.FalseValue
	}
	
	// 如果读到了数据，需要把它放回缓冲区（这里简化处理）
	return bytecode.TrueValue
}

// ============================================================================
// 数据读写函数
// ============================================================================

// nativeTcpWrite 写入字符串数据
func nativeTcpWrite(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	connID := args[0].AsInt()
	data := args[1].AsString()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewInt(-1)
	}
	n, err := tc.conn.Write([]byte(data))
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(n))
}

// nativeTcpWriteBytes 写入字节数组
func nativeTcpWriteBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	connID := args[0].AsInt()
	if args[1].Type != bytecode.ValBytes {
		return bytecode.NewInt(-1)
	}
	data := args[1].AsBytes()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewInt(-1)
	}
	n, err := tc.conn.Write(data)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	return bytecode.NewInt(int64(n))
}

// nativeTcpRead 读取字符串数据
func nativeTcpRead(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	length := int(args[1].AsInt())
	tc, ok := getConnection(connID)
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

// nativeTcpReadBytes 读取字节数组
func nativeTcpReadBytes(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes([]byte{})
	}
	connID := args[0].AsInt()
	length := int(args[1].AsInt())
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewBytes([]byte{})
	}
	buf := make([]byte, length)
	n, err := tc.reader.Read(buf)
	if err != nil {
		return bytecode.NewBytes([]byte{})
	}
	return bytecode.NewBytes(buf[:n])
}

// nativeTcpReadExact 精确读取指定长度的字节
func nativeTcpReadExact(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewBytes([]byte{})
	}
	connID := args[0].AsInt()
	length := int(args[1].AsInt())
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewBytes([]byte{})
	}
	buf := make([]byte, length)
	totalRead := 0
	for totalRead < length {
		n, err := tc.reader.Read(buf[totalRead:])
		if err != nil {
			if totalRead > 0 {
				return bytecode.NewBytes(buf[:totalRead])
			}
			return bytecode.NewBytes([]byte{})
		}
		totalRead += n
	}
	return bytecode.NewBytes(buf)
}

// nativeTcpReadLine 读取一行
func nativeTcpReadLine(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewString("")
	}
	line, err := tc.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(line)
}

// nativeTcpReadUntil 读取直到遇到指定分隔符
func nativeTcpReadUntil(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	delimiter := args[1].AsString()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewString("")
	}
	if len(delimiter) == 0 {
		return bytecode.NewString("")
	}
	data, err := tc.reader.ReadString(delimiter[0])
	if err != nil && len(data) == 0 {
		return bytecode.NewString("")
	}
	return bytecode.NewString(data)
}

// nativeTcpAvailable 获取缓冲区中可读的字节数
func nativeTcpAvailable(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(0)
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewInt(0)
	}
	return bytecode.NewInt(int64(tc.reader.Buffered()))
}

// nativeTcpFlush 刷新写入缓冲区（对于带缓冲的写入器）
func nativeTcpFlush(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	_, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	// net.Conn 的 Write 是无缓冲的，直接返回成功
	return bytecode.TrueValue
}

// ============================================================================
// 超时配置函数
// ============================================================================

// nativeTcpSetTimeout 设置通用超时（秒）
func nativeTcpSetTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	seconds := args[1].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	err := tc.conn.SetDeadline(deadline)
	return bytecode.NewBool(err == nil)
}

// nativeTcpSetTimeoutMs 设置通用超时（毫秒）
func nativeTcpSetTimeoutMs(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	ms := args[1].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	err := tc.conn.SetDeadline(deadline)
	return bytecode.NewBool(err == nil)
}

// nativeTcpSetReadTimeout 设置读取超时（毫秒）
func nativeTcpSetReadTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	ms := args[1].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	if ms <= 0 {
		// 清除超时
		err := tc.conn.SetReadDeadline(time.Time{})
		return bytecode.NewBool(err == nil)
	}
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	err := tc.conn.SetReadDeadline(deadline)
	return bytecode.NewBool(err == nil)
}

// nativeTcpSetWriteTimeout 设置写入超时（毫秒）
func nativeTcpSetWriteTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	ms := args[1].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	if ms <= 0 {
		// 清除超时
		err := tc.conn.SetWriteDeadline(time.Time{})
		return bytecode.NewBool(err == nil)
	}
	deadline := time.Now().Add(time.Duration(ms) * time.Millisecond)
	err := tc.conn.SetWriteDeadline(deadline)
	return bytecode.NewBool(err == nil)
}

// nativeTcpClearTimeout 清除所有超时设置
func nativeTcpClearTimeout(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	err := tc.conn.SetDeadline(time.Time{})
	return bytecode.NewBool(err == nil)
}

// ============================================================================
// Socket 选项配置函数
// ============================================================================

// nativeTcpSetKeepAlive 设置 KeepAlive
func nativeTcpSetKeepAlive(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	enabled := args[1].AsBool()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		// 可能是TLS连接，尝试获取底层连接
		if tlsConn, ok := tc.conn.(*tls.Conn); ok {
			// TLS连接无法直接设置KeepAlive，需要在建立连接前设置
			_ = tlsConn
			return bytecode.FalseValue
		}
		return bytecode.FalseValue
	}
	
	err := tcpConn.SetKeepAlive(enabled)
	if err != nil {
		return bytecode.FalseValue
	}
	
	// 如果提供了第三个参数，设置KeepAlive间隔
	if len(args) >= 3 && enabled {
		intervalSeconds := args[2].AsInt()
		if intervalSeconds > 0 {
			err = tcpConn.SetKeepAlivePeriod(time.Duration(intervalSeconds) * time.Second)
			if err != nil {
				return bytecode.FalseValue
			}
		}
	}
	
	return bytecode.TrueValue
}

// nativeTcpSetNoDelay 设置 NoDelay（禁用 Nagle 算法）
func nativeTcpSetNoDelay(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	enabled := args[1].AsBool()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		return bytecode.FalseValue
	}
	
	err := tcpConn.SetNoDelay(enabled)
	return bytecode.NewBool(err == nil)
}

// nativeTcpSetLinger 设置 Linger 选项
func nativeTcpSetLinger(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	seconds := int(args[1].AsInt())
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		return bytecode.FalseValue
	}
	
	err := tcpConn.SetLinger(seconds)
	return bytecode.NewBool(err == nil)
}

// nativeTcpSetReadBuffer 设置读取缓冲区大小
func nativeTcpSetReadBuffer(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	size := int(args[1].AsInt())
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		return bytecode.FalseValue
	}
	
	err := tcpConn.SetReadBuffer(size)
	if err == nil {
		tc.readBufSize = size
	}
	return bytecode.NewBool(err == nil)
}

// nativeTcpSetWriteBuffer 设置写入缓冲区大小
func nativeTcpSetWriteBuffer(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	size := int(args[1].AsInt())
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	
	tcpConn, ok := tc.conn.(*net.TCPConn)
	if !ok {
		return bytecode.FalseValue
	}
	
	err := tcpConn.SetWriteBuffer(size)
	if err == nil {
		tc.writeBufSize = size
	}
	return bytecode.NewBool(err == nil)
}

// ============================================================================
// 地址信息函数
// ============================================================================

// nativeTcpGetLocalAddr 获取本地地址
func nativeTcpGetLocalAddr(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewString("")
	}
	addr := tc.conn.LocalAddr()
	if addr == nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(addr.String())
}

// nativeTcpGetRemoteAddr 获取远程地址
func nativeTcpGetRemoteAddr(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewString("")
	}
	addr := tc.conn.RemoteAddr()
	if addr == nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(addr.String())
}

// nativeTcpGetLocalHost 获取本地主机地址
func nativeTcpGetLocalHost(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewString("")
	}
	addr := tc.conn.LocalAddr()
	if addr == nil {
		return bytecode.NewString("")
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(host)
}

// nativeTcpGetLocalPort 获取本地端口
func nativeTcpGetLocalPort(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(0)
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewInt(0)
	}
	addr := tc.conn.LocalAddr()
	if addr == nil {
		return bytecode.NewInt(0)
	}
	_, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return bytecode.NewInt(0)
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return bytecode.NewInt(int64(port))
}

// nativeTcpGetRemoteHost 获取远程主机地址
func nativeTcpGetRemoteHost(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewString("")
	}
	addr := tc.conn.RemoteAddr()
	if addr == nil {
		return bytecode.NewString("")
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return bytecode.NewString("")
	}
	return bytecode.NewString(host)
}

// nativeTcpGetRemotePort 获取远程端口
func nativeTcpGetRemotePort(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewInt(0)
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.NewInt(0)
	}
	addr := tc.conn.RemoteAddr()
	if addr == nil {
		return bytecode.NewInt(0)
	}
	_, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return bytecode.NewInt(0)
	}
	var port int
	fmt.Sscanf(portStr, "%d", &port)
	return bytecode.NewInt(int64(port))
}

// nativeTcpIsTLS 检查是否为TLS连接
func nativeTcpIsTLS(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok {
		return bytecode.FalseValue
	}
	return bytecode.NewBool(tc.isTLS)
}

// ============================================================================
// TLS/SSL 函数
// ============================================================================

// nativeTlsConnect TLS安全连接
func nativeTlsConnect(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	host := args[0].AsString()
	port := args[1].AsInt()
	
	// 获取超时时间（可选，默认10秒）
	timeout := 10 * time.Second
	if len(args) >= 3 {
		timeoutMs := args[2].AsInt()
		if timeoutMs > 0 {
			timeout = time.Duration(timeoutMs) * time.Millisecond
		}
	}
	
	address := fmt.Sprintf("%s:%d", host, port)
	
	// 创建带超时的dialer
	dialer := &net.Dialer{Timeout: timeout}
	
	// TLS配置
	tlsConfig := &tls.Config{
		ServerName: host,
	}
	
	conn, err := tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	
	connID := registerConnection(conn, true)
	return bytecode.NewInt(connID)
}

// nativeTlsConnectInsecure TLS连接（跳过证书验证，仅用于测试）
func nativeTlsConnectInsecure(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.NewInt(-1)
	}
	host := args[0].AsString()
	port := args[1].AsInt()
	
	// 获取超时时间（可选，默认10秒）
	timeout := 10 * time.Second
	if len(args) >= 3 {
		timeoutMs := args[2].AsInt()
		if timeoutMs > 0 {
			timeout = time.Duration(timeoutMs) * time.Millisecond
		}
	}
	
	address := fmt.Sprintf("%s:%d", host, port)
	
	// 创建带超时的dialer
	dialer := &net.Dialer{Timeout: timeout}
	
	// TLS配置（跳过证书验证）
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	
	conn, err := tls.DialWithDialer(dialer, "tcp", address, tlsConfig)
	if err != nil {
		return bytecode.NewInt(-1)
	}
	
	connID := registerConnection(conn, true)
	return bytecode.NewInt(connID)
}

// nativeTlsUpgrade 将现有TCP连接升级为TLS
func nativeTlsUpgrade(args []bytecode.Value) bytecode.Value {
	if len(args) < 2 {
		return bytecode.FalseValue
	}
	connID := args[0].AsInt()
	serverName := args[1].AsString()
	
	tcpConnMutex.Lock()
	tc, ok := tcpConnections[connID]
	if !ok {
		tcpConnMutex.Unlock()
		return bytecode.FalseValue
	}
	
	if tc.isTLS {
		tcpConnMutex.Unlock()
		return bytecode.TrueValue // 已经是TLS连接
	}
	
	// TLS配置
	tlsConfig := &tls.Config{
		ServerName: serverName,
	}
	
	// 跳过验证选项
	if len(args) >= 3 && args[2].AsBool() {
		tlsConfig.InsecureSkipVerify = true
	}
	
	// 升级连接
	tlsConn := tls.Client(tc.conn, tlsConfig)
	err := tlsConn.Handshake()
	if err != nil {
		tcpConnMutex.Unlock()
		return bytecode.FalseValue
	}
	
	// 更新连接
	tc.conn = tlsConn
	tc.reader = bufio.NewReader(tlsConn)
	tc.isTLS = true
	tcpConnMutex.Unlock()
	
	return bytecode.TrueValue
}

// nativeTlsGetVersion 获取TLS版本
func nativeTlsGetVersion(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok || !tc.isTLS {
		return bytecode.NewString("")
	}
	
	tlsConn, ok := tc.conn.(*tls.Conn)
	if !ok {
		return bytecode.NewString("")
	}
	
	state := tlsConn.ConnectionState()
	switch state.Version {
	case tls.VersionTLS10:
		return bytecode.NewString("TLS 1.0")
	case tls.VersionTLS11:
		return bytecode.NewString("TLS 1.1")
	case tls.VersionTLS12:
		return bytecode.NewString("TLS 1.2")
	case tls.VersionTLS13:
		return bytecode.NewString("TLS 1.3")
	default:
		return bytecode.NewString("Unknown")
	}
}

// nativeTlsGetCipherSuite 获取TLS加密套件
func nativeTlsGetCipherSuite(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok || !tc.isTLS {
		return bytecode.NewString("")
	}
	
	tlsConn, ok := tc.conn.(*tls.Conn)
	if !ok {
		return bytecode.NewString("")
	}
	
	state := tlsConn.ConnectionState()
	return bytecode.NewString(tls.CipherSuiteName(state.CipherSuite))
}

// nativeTlsGetServerName 获取TLS服务器名称
func nativeTlsGetServerName(args []bytecode.Value) bytecode.Value {
	if len(args) < 1 {
		return bytecode.NewString("")
	}
	connID := args[0].AsInt()
	tc, ok := getConnection(connID)
	if !ok || !tc.isTLS {
		return bytecode.NewString("")
	}
	
	tlsConn, ok := tc.conn.(*tls.Conn)
	if !ok {
		return bytecode.NewString("")
	}
	
	state := tlsConn.ConnectionState()
	return bytecode.NewString(state.ServerName)
}
