package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

var (
	mu     sync.RWMutex
	Global = DefaultConfig()
)

type Config struct {
	Services ServicesConfig `json:"services"`
	Logging  LoggingConfig  `json:"logging"`
	Auth     AuthConfig     `json:"auth"`
	TLS      TLSConfig      `json:"tls"`
}

type ServicesConfig struct {
	HTTPProxy   ServiceConfig `json:"http_proxy"`
	SOCKS5Proxy ServiceConfig `json:"socks5_proxy"`
	Admin       ServiceConfig `json:"admin"`
	Gateway     ServiceConfig `json:"gateway"`
}

type ServiceConfig struct {
	Enabled    bool   `json:"enabled"`
	Bind       string `json:"bind"`
	Advertise  string `json:"advertise,omitempty"`
}

type LoggingConfig struct {
	Level  string `json:"level"`  // debug, info, warn, error
	Output string `json:"output"` // stdout, file
	File   string `json:"file,omitempty"`
}

type AuthConfig struct {
	Enabled  bool     `json:"enabled"`
	Users    []User   `json:"users,omitempty"`
}

type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type TLSConfig struct {
	Enabled  bool   `json:"enabled"`
	CertFile string `json:"cert_file,omitempty"`
	KeyFile  string `json:"key_file,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{
		Services: ServicesConfig{
			HTTPProxy: ServiceConfig{
				Enabled: true,
				Bind:    ":8080",
			},
			SOCKS5Proxy: ServiceConfig{
				Enabled: true,
				Bind:    ":1080",
			},
			Admin: ServiceConfig{
				Enabled: true,
				Bind:    ":8088",
			},
			Gateway: ServiceConfig{
				Enabled: false,
				Bind:    ":8081",
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Output: "stdout",
		},
	}
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	mu.Lock()
	Global = cfg
	mu.Unlock()

	return cfg, nil
}

// Save 保存配置到文件
func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// Get 线程安全获取全局配置
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return Global
}
