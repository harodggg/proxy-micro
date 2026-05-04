#!/bin/bash
# Proxy Micro - 微服务代理工具
set -e

DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$DIR/bin"
CONFIG="$DIR/config.json"

export PATH="$PATH:/usr/local/go/bin"

# 主菜单
menu() {
  echo ""
  echo "╔══════════════════════════════════════╗"
  echo "║   ⚡ Proxy Micro  微服务代理工具     ║"
  echo "╠══════════════════════════════════════╣"
  echo "║  1) 启动所有服务                     ║"
  echo "║  2) 启动 HTTP Proxy (端口 :8080)    ║"
  echo "║  3) 启动 SOCKS5 Proxy (端口 :1080)   ║"
  echo "║  4) 启动 Admin 管理面板 (端口 :8088) ║"
  echo "║  5) 停止所有服务                     ║"
  echo "║  6) 查看状态                         ║"
  echo "║  7) 编译                             ║"
  echo "║  0) 退出                             ║"
  echo "╚══════════════════════════════════════╝"
  echo ""
  read -p "选择操作 [0-7]: " choice
  
  case $choice in
    1) start_all ;;
    2) start_service "http-proxy" ;;
    3) start_service "socks5-proxy" ;;
    4) start_service "admin" ;;
    5) stop_all ;;
    6) show_status ;;
    7) build_all ;;
    0) exit 0 ;;
    *) echo "无效选择" ;;
  esac
  menu
}

build_all() {
  echo "📦 编译所有服务..."
  cd "$DIR"
  go build -o "$BIN/http-proxy" ./cmd/http-proxy
  go build -o "$BIN/socks5-proxy" ./cmd/socks5-proxy
  go build -o "$BIN/admin" ./cmd/admin
  echo "✅ 编译完成"
}

start_service() {
  local name=$1
  if pgrep -f "bin/$name" > /dev/null; then
    echo "⚠️  $name 已在运行中"
    return
  fi
  
  if [ ! -f "$BIN/$name" ]; then
    echo "📦 编译 $name..."
    cd "$DIR"
    go build -o "$BIN/$name" "./cmd/$name"
  fi
  
  cd "$DIR"
  nohup "$BIN/$name" "$CONFIG" > "/tmp/$name.log" 2>&1 &
  sleep 1
  echo "✅ $name 已启动 (PID: $!)"
}

start_all() {
  echo "🚀 启动所有服务..."
  start_service "http-proxy"
  start_service "socks5-proxy"
  start_service "admin"
  echo ""
  echo "📊 HTTP Proxy:   http://0.0.0.0:8080"
  echo "🧦 SOCKS5 Proxy: socks5://0.0.0.0:1080"
  echo "📋 Admin:        http://0.0.0.0:8088"
}

stop_all() {
  echo "⏹  停止所有服务..."
  pkill -f "bin/http-proxy" 2>/dev/null || true
  pkill -f "bin/socks5-proxy" 2>/dev/null || true
  pkill -f "bin/admin" 2>/dev/null || true
  echo "✅ 已停止"
}

show_status() {
  echo ""
  echo "=== 服务状态 ==="
  for svc in "http-proxy:HTTP Proxy" "socks5-proxy:SOCKS5 Proxy" "admin:Admin"; do
    name="${svc%%:*}"
    label="${svc##*:}"
    if pgrep -f "bin/$name" > /dev/null; then
      echo "  ✅ $label  运行中"
    else
      echo "  ❌ $label  未运行"
    fi
  done
  echo ""
  echo "=== 端口监听 ==="
  ss -tlnp | grep -E '(8080|1080|8088)' || echo "(无)"
}

# 确保二进制目录存在
mkdir -p "$BIN"

if [ "$1" = "start" ]; then
  start_all
elif [ "$1" = "stop" ]; then
  stop_all
elif [ "$1" = "status" ]; then
  show_status
elif [ "$1" = "build" ]; then
  build_all
else
  menu
fi
