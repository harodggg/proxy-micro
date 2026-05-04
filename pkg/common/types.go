package common

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// ProxyType 代理协议类型
type ProxyType string

const (
	ProxyHTTP   ProxyType = "http"
	ProxySOCKS5 ProxyType = "socks5"
)

// ProxySession 代理会话信息
type ProxySession struct {
	ID        string    `json:"id"`
	Type      ProxyType `json:"type"`
	ClientAddr string   `json:"client_addr"`
	TargetAddr string   `json:"target_addr"`
	BytesIn   int64     `json:"bytes_in"`
	BytesOut  int64     `json:"bytes_out"`
	StartedAt time.Time `json:"started_at"`
	ClosedAt  time.Time `json:"closed_at,omitempty"`
}

// TrafficStats 流量统计
type TrafficStats struct {
	TotalBytesIn  int64            `json:"total_bytes_in"`
	TotalBytesOut int64            `json:"total_bytes_out"`
	ActiveConns   int              `json:"active_conns"`
	TotalConns    int64            `json:"total_conns"`
	ByType        map[ProxyType]*ServiceStats `json:"by_type"`
}

type ServiceStats struct {
	BytesIn  int64 `json:"bytes_in"`
	BytesOut int64 `json:"bytes_out"`
	Conns    int64 `json:"conns"`
}

// ResolveAddr 解析目标地址
func ResolveAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid address %s: %w", addr, err)
	}
	if net.ParseIP(host) != nil {
		return addr, nil
	}
	// 尝试 DNS 解析（只验证，不做缓存）
	ips, err := net.LookupIP(host)
	if err != nil {
		return "", fmt.Errorf("dns lookup failed for %s: %w", host, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for %s", host)
	}
	return net.JoinHostPort(ips[0].String(), port), nil
}

// MustParseAddr 安全的地址解析（兼容无端口的情况）
func MustParseAddr(addr string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, fmt.Errorf("cannot parse %s: %w", addr, err)
	}
	port := 0
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}
	return host, port, nil
}

// JSON 序列化辅助
func ToJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
