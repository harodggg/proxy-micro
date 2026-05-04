package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/proxy-micro/pkg/config"
	"github.com/proxy-micro/pkg/protocol"
	"github.com/proxy-micro/pkg/stats"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if !cfg.Services.SOCKS5Proxy.Enabled {
		log.Println("SOCKS5 proxy service is disabled")
		return
	}

	tracker := &stats.Tracker{}

	// 启动统计 HTTP 服务（内部端口）
	go func() {
		statsAddr := ":1089"
		log.Printf("📊 [SOCKS5 Stats] listening on %s", statsAddr)
		if err := http.ListenAndServe(statsAddr, tracker.Handler()); err != nil {
			log.Printf("stats server error: %v", err)
		}
	}()

	bind := cfg.Services.SOCKS5Proxy.Bind
	if bind == "" {
		bind = ":1080"
	}

	listener, err := net.Listen("tcp", bind)
	if err != nil {
		log.Fatalf("listen %s: %v", bind, err)
	}
	defer listener.Close()

	fmt.Printf("🧦 [SOCKS5 Proxy] listening on %s\n", bind)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		fmt.Println("\n⏹  SOCKS5 Proxy shutting down...")
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			break
		}

		go func(c net.Conn) {
			addr := c.RemoteAddr().String()
			tracker.AddConn()
			defer tracker.DoneConn()

			log.Printf("→ [SOCKS5] new connection from %s", addr)

			wc := &countingConn{Conn: c, tracker: tracker}

			var auth protocol.Socks5AuthFunc
			if cfg.Auth.Enabled {
				auth = func(username, password string) bool {
					for _, u := range cfg.Auth.Users {
						if u.Username == username && u.Password == password {
							return true
						}
					}
					return false
				}
			}

			if err := protocol.HandleSocks5(wc, auth); err != nil {
				log.Printf("← [SOCKS5] %s done: %v", addr, err)
			}
		}(conn)
	}
}

// countingConn 包装 net.Conn 追踪流量
type countingConn struct {
	net.Conn
	tracker *stats.Tracker
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.tracker.AddBytes(int64(n), 0)
	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.tracker.AddBytes(0, int64(n))
	return n, err
}

var _ io.ReadWriter = (*countingConn)(nil)
