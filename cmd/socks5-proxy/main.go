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
			log.Printf("→ [SOCKS5] new connection from %s", addr)

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

			if err := protocol.HandleSocks5(c, auth); err != nil {
				log.Printf("← [SOCKS5] %s done: %v", addr, err)
			}
		}(conn)
	}
}
