# Proxy Micro ⚡

微服务架构的代理工具——HTTP 代理 + SOCKS5 代理 + 管理面板。

## 架构

```
proxy-micro/
├── cmd/
│   ├── http-proxy/     # HTTP/HTTPS 代理服务 (CONNECT + Forward)
│   ├── socks5-proxy/   # SOCKS5 代理服务
│   └── admin/          # 管理面板 + REST API
├── pkg/
│   ├── config/         # 配置管理
│   ├── protocol/       # 协议实现 (HTTP/SOCKS5)
│   └── common/         # 通用类型和工具
├── docker-compose.yml  # Docker 部署
└── config.json         # 配置文件
```

## 快速开始

### 直接运行

```bash
# 编译并启动所有服务
chmod +x start.sh
./start.sh

# 或者一键启动
./start.sh start
```

### 测试代理

```bash
# HTTP 代理测试
curl -x http://127.0.0.1:8080 http://example.com

# HTTPS 通过 HTTP CONNECT
curl -x http://127.0.0.1:8080 https://httpbin.org/ip

# SOCKS5 代理测试
curl --socks5 127.0.0.1:1080 https://httpbin.org/ip
```

### 管理面板

打开浏览器访问 `http://<IP>:8088` 查看实时统计。

### Docker 部署

```bash
docker-compose up -d
```

## 配置

编辑 `config.json`：

```json
{
  "services": {
    "http_proxy":  { "enabled": true, "bind": ":8080" },
    "socks5_proxy": { "enabled": true, "bind": ":1080" },
    "admin":       { "enabled": true, "bind": ":8088" }
  },
  "auth": {
    "enabled": false,
    "users": [{ "username": "admin", "password": "proxy123" }]
  }
}
```

## 服务端口

| 服务 | 端口 | 说明 |
|------|------|------|
| HTTP Proxy | 8080 | HTTP/HTTPS 代理 |
| SOCKS5 Proxy | 1080 | SOCKS5 代理 |
| Admin | 8088 | 管理面板 + API |
