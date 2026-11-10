package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"invoice/server/internal/api"
	"invoice/server/internal/config"
	"invoice/server/internal/ocr"
	"invoice/server/internal/storage"
	"invoice/server/internal/store"
)

func main() {
	log.SetFlags(log.Ltime)
	cfg := config.Load()

	if err := store.Open(cfg.DSN); err != nil {
		log.Fatalf("数据库初始化失败：%v", err)
	}

	srv := api.New(cfg)
	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go maintenance(cfg)
	banner(cfg)

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		log.Fatalf("服务启动失败：%v", err)
	case <-quit:
		log.Println("正在停止服务 ...")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

// maintenance 定期清理临时文件与过期会话；每天凌晨执行一次保留期清理。
func maintenance(cfg *config.Config) {
	lastCleanupDay := ""
	for {
		time.Sleep(30 * time.Minute)
		storage.CleanupTemp(cfg, 12*time.Hour)
		today := time.Now().Format("2006-01-02")
		if today != lastCleanupDay && time.Now().Hour() == 3 {
			if settings := store.GetSettings(); settings["retention_mode"] == "auto" {
				log.Println("执行保留期清理 ...")
			}
			lastCleanupDay = today
		}
	}
}

func banner(cfg *config.Config) {
	engine := ocr.CurrentEngine()
	ocrState := "不可用（只能读取 PDF 文字层）"
	if engine.Available {
		ocrState = fmt.Sprintf("%s（%s）", engine.Label, engine.Detail)
	} else if engine.Detail != "" {
		ocrState = "不可用：" + engine.Detail
	}
	ip := localIP()
	line := "────────────────────────────────────────────────────────"
	fmt.Printf("\n  %s\n", line)
	fmt.Println("   票管家 · 发票报销管理平台  v1.0.1")
	fmt.Printf("  %s\n", line)
	fmt.Printf("   本机访问 :  http://127.0.0.1%s\n", portSuffix(cfg.Addr))
	fmt.Printf("   局域网   :  http://%s%s\n", ip, portSuffix(cfg.Addr))
	fmt.Printf("   数据目录 :  %s\n", mustAbs(cfg.DataDir))
	fmt.Printf("   前端目录 :  %s\n", cfg.WebDir)
	fmt.Printf("   离线识别 :  %s\n", ocrState)
	fmt.Printf("   运行平台 :  %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  %s\n\n", line)
}

func portSuffix(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ":8686"
	}
	return ":" + port
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	fallback := ""
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
			continue
		}
		ip := ipnet.IP.String()
		// 优先返回常见的局域网地址，避免选到 169.254 链路本地地址
		if strings.HasPrefix(ip, "192.168.") || strings.HasPrefix(ip, "10.") ||
			strings.HasPrefix(ip, "172.16.") || strings.HasPrefix(ip, "172.17.") ||
			strings.HasPrefix(ip, "172.18.") || strings.HasPrefix(ip, "172.19.") ||
			strings.HasPrefix(ip, "172.2") || strings.HasPrefix(ip, "172.3") {
			return ip
		}
		if fallback == "" {
			fallback = ip
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}
