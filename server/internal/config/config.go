package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Config 保存服务运行期的全部可配置项。
type Config struct {
	Addr    string // 监听地址，例如 :8686
	DSN     string // MySQL DSN
	DataDir string // 数据目录（附件、临时文件、备份）
	WebDir  string // 前端构建产物目录
	Secret  []byte // JWT 签名密钥
	Debug   bool
}

var current *Config

// Get 返回全局配置。
func Get() *Config { return current }

// Load 解析命令行参数与环境变量，优先级：命令行 > 环境变量 > 默认值。
func Load() *Config {
	c := &Config{}
	flag.StringVar(&c.Addr, "addr", env("INVOICE_ADDR", ":8686"), "监听地址，例如 :8686")
	flag.StringVar(&c.DSN, "dsn", env("INVOICE_DSN", ""), "MySQL DSN")
	flag.StringVar(&c.DataDir, "data", env("INVOICE_DATA", "data"), "数据目录")
	flag.StringVar(&c.WebDir, "web", env("INVOICE_WEB", "../web/dist"), "前端静态资源目录")
	flag.BoolVar(&c.Debug, "debug", env("INVOICE_DEBUG", "") == "1", "输出调试日志")
	flag.Parse()

	if c.DSN == "" {
		c.DSN = "invoice:Invoice@2026@tcp(127.0.0.1:3306)/invoice?charset=utf8mb4&parseTime=true&loc=Local&multiStatements=true"
	}
	abs, err := filepath.Abs(c.WebDir)
	if err == nil {
		c.WebDir = abs
	}
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		log.Fatalf("无法创建数据目录 %s: %v", c.DataDir, err)
	}
	for _, sub := range []string{"files", "tmp", "thumbs", "logs"} {
		_ = os.MkdirAll(filepath.Join(c.DataDir, sub), 0o755)
	}
	c.Secret = loadSecret(filepath.Join(c.DataDir, "jwt.secret"))
	current = c
	return c
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// loadSecret 首次运行时生成随机密钥并持久化，保证重启后 token 仍有效。
func loadSecret(path string) []byte {
	if data, err := os.ReadFile(path); err == nil && len(data) >= 32 {
		return data
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("生成签名密钥失败: %v", err)
	}
	encoded := []byte(hex.EncodeToString(buf))
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		log.Printf("警告：签名密钥无法持久化（%v），重启后需要重新登录", err)
	}
	return encoded
}

// Path 拼接数据目录下的子路径。
func (c *Config) Path(parts ...string) string {
	return filepath.Join(append([]string{c.DataDir}, parts...)...)
}
