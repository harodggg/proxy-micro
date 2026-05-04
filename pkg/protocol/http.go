package protocol

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// HTTPAuthFunc HTTP 代理认证回调
type HTTPAuthFunc func(username, password string) bool

// HandleHTTPProxy 处理 HTTP 代理连接
func HandleHTTPProxy(conn net.Conn, auth HTTPAuthFunc) error {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return fmt.Errorf("read request: %w", err)
	}

	// 认证检查
	if auth != nil {
		user, pass, ok := parseBasicAuth(req.Header.Get("Proxy-Authorization"))
		if !ok || !auth(user, pass) {
			resp := &http.Response{
				StatusCode: http.StatusProxyAuthRequired,
				ProtoMajor: 1,
				ProtoMinor: 1,
				Header:     make(http.Header),
			}
			resp.Header.Set("Proxy-Authenticate", "Basic realm=\"proxy\"")
			resp.Header.Set("Content-Length", "0")
			resp.Write(conn)
			return fmt.Errorf("auth failed for %s", req.URL.Host)
		}
	}

	target := req.URL.Host
	if !strings.Contains(target, ":") {
		target = target + ":80"
	}

	// 处理 CONNECT 方法（HTTPS 隧道）
	if req.Method == http.MethodConnect {
		return handleHTTPConnect(conn, target)
	}

	// 普通 HTTP 代理
	return handleHTTPForward(conn, req, target)
}

// handleHTTPConnect 处理 CONNECT 隧道
func handleHTTPConnect(conn net.Conn, target string) error {
	targetConn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		resp := &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			ProtoMajor: 1,
			ProtoMinor: 1,
		}
		resp.Header = make(http.Header)
		resp.Header.Set("Content-Length", "0")
		resp.Write(conn)
		return fmt.Errorf("dial %s: %w", target, err)
	}
	defer targetConn.Close()

	// 发送 200 Connection Established
	conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	// 双向转发
	relay(conn, targetConn)
	return nil
}

// handleHTTPForward 处理 HTTP 前向代理
func handleHTTPForward(conn net.Conn, req *http.Request, target string) error {
	targetConn, err := net.DialTimeout("tcp", target, 10*time.Second)
	if err != nil {
		resp := &http.Response{
			StatusCode: http.StatusBadGateway,
			ProtoMajor: 1,
			ProtoMinor: 1,
		}
		resp.Header = make(http.Header)
		resp.Header.Set("Content-Length", "0")
		resp.Write(conn)
		return fmt.Errorf("dial %s: %w", target, err)
	}
	defer targetConn.Close()

	// 重写请求：去掉绝对 URL，保留相对路径
	req.RequestURI = req.URL.Path
	if req.URL.RawQuery != "" {
		req.RequestURI = req.URL.Path + "?" + req.URL.RawQuery
	}

	// 移除代理相关的请求头
	req.Header.Del("Proxy-Connection")
	req.Header.Del("Proxy-Authorization")

	// 转发请求到目标服务器
	if err := req.Write(targetConn); err != nil {
		return fmt.Errorf("write request: %w", err)
	}

	// 读取响应并转发回客户端
	resp, err := http.ReadResponse(bufio.NewReader(targetConn), req)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()

	resp.Write(conn)
	return nil
}

// parseBasicAuth 解析 HTTP Basic Auth
func parseBasicAuth(auth string) (user, pass string, ok bool) {
	if auth == "" {
		return "", "", false
	}

	const prefix = "Basic "
	if !strings.HasPrefix(auth, prefix) {
		return "", "", false
	}

	// 简单 base64 解码
	decoded := simpleBase64Decode(auth[len(prefix):])
	if decoded == "" {
		return "", "", false
	}

	idx := strings.IndexByte(decoded, ':')
	if idx < 0 {
		return "", "", false
	}

	return decoded[:idx], decoded[idx+1:], true
}

// simpleBase64Decode 简易 base64 解码（无 padding 处理）
func simpleBase64Decode(s string) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	// 去掉填充字符
	s = strings.TrimRight(s, "=")

	if len(s) == 0 {
		return ""
	}

	// 每个字符映射到 6 bit
	var result []byte
	buffer := 0
	bitsRemaining := 0

	for _, ch := range s {
		idx := strings.IndexRune(alphabet, ch)
		if idx < 0 {
			return ""
		}
		buffer = (buffer << 6) | idx
		bitsRemaining += 6
		if bitsRemaining >= 8 {
			bitsRemaining -= 8
			result = append(result, byte(buffer>>bitsRemaining))
			buffer &= (1 << bitsRemaining) - 1
		}
	}

	return string(result)
}

// TLSConfig TLS 配置
type TLSConfig struct {
	CertFile string
	KeyFile  string
}

// ListenTLSOrPlain 根据配置选择监听 TLS 或普通 TCP
func ListenTLSOrPlain(addr string, tlsCfg *TLSConfig) (net.Listener, error) {
	if tlsCfg != nil && tlsCfg.CertFile != "" && tlsCfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load tls cert: %w", err)
		}
		return tls.Listen("tcp", addr, &tls.Config{
			Certificates: []tls.Certificate{cert},
		})
	}
	return net.Listen("tcp", addr)
}
