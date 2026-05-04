package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/proxy-micro/pkg/config"
	"github.com/proxy-micro/pkg/protocol"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// 加载配置
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if !cfg.Services.HTTPProxy.Enabled {
		log.Println("HTTP proxy service is disabled")
		return
	}

	bind := cfg.Services.HTTPProxy.Bind
	if bind == "" {
		bind = ":8080"
	}

	listener, err := net.Listen("tcp", bind)
	if err != nil {
		log.Fatalf("listen %s: %v", bind, err)
	}
	defer listener.Close()

	fmt.Printf("🔄 [HTTP Proxy] listening on %s\n", bind)

	// 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		fmt.Println("\n⏹  HTTP Proxy shutting down...")
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			break
		}

		go func(c net.Conn) {
			addr := c.RemoteAddr().String()
			log.Printf("→ [HTTP] new connection from %s", addr)

			var auth protocol.HTTPAuthFunc
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

			if err := protocol.HandleHTTPProxy(c, auth); err != nil {
				log.Printf("← [HTTP] %s done: %v", addr, err)
			}
		}(conn)
	}
}
