package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
)

// SOCKS5 协议常量
const (
	SOCKS5Version = 0x05

	SOCKS5AuthNone     = 0x00
	SOCKS5AuthPassword = 0x02
	SOCKS5AuthNoAccept = 0xFF

	SOCKS5CmdConnect = 0x01
	SOCKS5CmdUDP     = 0x03

	SOCKS5ATYPIPv4   = 0x01
	SOCKS5ATYPDomain = 0x03
	SOCKS5ATYPIPv6   = 0x04

	SOCKS5ReplySuccess             = 0x00
	SOCKS5ReplyFailure             = 0x01
	SOCKS5ReplyNotAllowed          = 0x02
	SOCKS5ReplyNetworkUnreachable  = 0x03
	SOCKS5ReplyHostUnreachable     = 0x04
	SOCKS5ReplyConnectionRefused   = 0x05
	SOCKS5ReplyTTLExpired          = 0x06
	SOCKS5ReplyCmdNotSupported     = 0x07
	SOCKS5ReplyAddrTypeNotSupported = 0x08
)

var (
	ErrVersionMismatch  = errors.New("socks5: socks version mismatch")
	ErrAuthFailed       = errors.New("socks5: authentication failed")
	ErrCmdNotSupported  = errors.New("socks5: command not supported")
	ErrInvalidAddrType  = errors.New("socks5: invalid address type")
)

// Socks5AuthFunc 用户认证回调
type Socks5AuthFunc func(username, password string) bool

// HandleSocks5 处理 SOCKS5 代理连接
func HandleSocks5(conn net.Conn, auth Socks5AuthFunc) error {
	defer conn.Close()

	// 1. 握手：协商认证方式
	if err := socks5Handshake(conn, auth != nil); err != nil {
		return fmt.Errorf("handshake: %w", err)
	}

	// 2. 认证
	if auth != nil {
		if err := socks5Authenticate(conn, auth); err != nil {
			return err
		}
	}

	// 3. 获取请求
	target, err := socks5GetRequest(conn)
	if err != nil {
		return fmt.Errorf("get request: %w", err)
	}

	// 4. 连接目标
	targetConn, err := net.Dial("tcp", target)
	if err != nil {
		socks5SendReply(conn, SOCKS5ReplyHostUnreachable, nil)
		return fmt.Errorf("dial %s: %w", target, err)
	}
	defer targetConn.Close()

	// 发送成功回复
	localAddr := targetConn.LocalAddr().(*net.TCPAddr)
	socks5SendReply(conn, SOCKS5ReplySuccess, localAddr)

	// 5. 双向转发
	relay(conn, targetConn)
	return nil
}

func socks5Handshake(conn net.Conn, needAuth bool) error {
	buf := make([]byte, 257)

	// 读取版本和方法数
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if buf[0] != SOCKS5Version {
		return ErrVersionMismatch
	}

	nMethods := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:nMethods]); err != nil {
		return fmt.Errorf("read methods: %w", err)
	}

	// 选择认证方式
	chosen := byte(SOCKS5AuthNoAccept)
	if needAuth {
		for i := 0; i < nMethods; i++ {
			if buf[i] == SOCKS5AuthPassword {
				chosen = SOCKS5AuthPassword
				break
			}
		}
	} else {
		for i := 0; i < nMethods; i++ {
			if buf[i] == SOCKS5AuthNone {
				chosen = SOCKS5AuthNone
				break
			}
		}
	}

	conn.Write([]byte{SOCKS5Version, chosen})
	return nil
}

func socks5Authenticate(conn net.Conn, auth Socks5AuthFunc) error {
	buf := make([]byte, 513)

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return fmt.Errorf("read auth header: %w", err)
	}

	if buf[0] != 0x01 {
		conn.Write([]byte{0x01, 0x01})
		return ErrAuthFailed
	}

	uLen := int(buf[1])
	if _, err := io.ReadFull(conn, buf[:uLen]); err != nil {
		return fmt.Errorf("read username: %w", err)
	}
	username := string(buf[:uLen])

	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		return fmt.Errorf("read pwd len: %w", err)
	}
	pLen := int(buf[0])
	if _, err := io.ReadFull(conn, buf[:pLen]); err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	password := string(buf[:pLen])

	if auth(username, password) {
		conn.Write([]byte{0x01, 0x00})
		return nil
	}

	conn.Write([]byte{0x01, 0x01})
	return ErrAuthFailed
}

func socks5GetRequest(conn net.Conn) (string, error) {
	buf := make([]byte, 263)

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return "", fmt.Errorf("read request: %w", err)
	}

	if buf[0] != SOCKS5Version {
		return "", ErrVersionMismatch
	}
	if buf[1] != SOCKS5CmdConnect {
		socks5SendReply(conn, SOCKS5ReplyCmdNotSupported, nil)
		return "", ErrCmdNotSupported
	}

	// 解析目标地址
	var host string
	switch buf[3] {
	case SOCKS5ATYPIPv4:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return "", fmt.Errorf("read ipv4: %w", err)
		}
		host = net.IP(buf[:4]).String()
	case SOCKS5ATYPDomain:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return "", fmt.Errorf("read domain len: %w", err)
		}
		dLen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:dLen]); err != nil {
			return "", fmt.Errorf("read domain: %w", err)
		}
		host = string(buf[:dLen])
	case SOCKS5ATYPIPv6:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return "", fmt.Errorf("read ipv6: %w", err)
		}
		host = net.IP(buf[:16]).String()
	default:
		socks5SendReply(conn, SOCKS5ReplyAddrTypeNotSupported, nil)
		return "", ErrInvalidAddrType
	}

	// 读取端口
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return "", fmt.Errorf("read port: %w", err)
	}
	port := binary.BigEndian.Uint16(buf[:2])

	return net.JoinHostPort(host, strconv.Itoa(int(port))), nil
}

func socks5SendReply(conn net.Conn, reply byte, addr *net.TCPAddr) {
	msg := make([]byte, 10)
	msg[0] = SOCKS5Version
	msg[1] = reply
	msg[2] = 0x00
	msg[3] = SOCKS5ATYPIPv4

	if addr != nil {
		ip4 := addr.IP.To4()
		if ip4 != nil {
			copy(msg[4:8], ip4)
		}
		binary.BigEndian.PutUint16(msg[8:10], uint16(addr.Port))
	}

	conn.Write(msg)
}

// relay 双向流量转发
func relay(a, b net.Conn) {
	done := make(chan struct{}, 2)

	copy := func(dst, src net.Conn) {
		io.Copy(dst, src)
		done <- struct{}{}
	}

	go copy(a, b)
	go copy(b, a)

	<-done
}
