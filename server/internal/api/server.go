package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"invoice/server/internal/auth"
	"invoice/server/internal/config"
	"invoice/server/internal/store"
)

// Server 承载全部 HTTP 路由。
type Server struct {
	cfg *config.Config
	mux *http.ServeMux
}

type ctxKey string

const userKey ctxKey = "user"

// New 构建路由表。
func New(cfg *config.Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	go s.backgroundMaintenance()
	return s
}

// backgroundMaintenance 每 6 小时检查一次保留期，到期执行自动清理。
func (s *Server) backgroundMaintenance() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		if store.GetSetting("retention_mode") != "auto" {
			continue
		}
		result := s.runRetention()
		store.Audit(nil, "system", "retention_cleanup", "system", fmt.Sprintf("%v", result), "")
		log.Printf("保留期清理完成：%v", result)
	}
}

// Handler 返回带中间件的 http.Handler。
func (s *Server) Handler() http.Handler {
	return s.withRecover(s.withLog(s.mux))
}

func (s *Server) routes() {
	m := s.mux

	// 公开接口
	m.HandleFunc("GET /api/bootstrap", s.handleBootstrap)
	m.HandleFunc("POST /api/setup", s.handleSetup)
	m.HandleFunc("POST /api/login", s.handleLogin)
	m.HandleFunc("GET /api/health", s.handleHealth)

	// 需要登录
	m.Handle("GET /api/me", s.auth(s.handleMe))
	m.Handle("POST /api/me/password", s.auth(s.handleChangePassword))
	m.Handle("POST /api/logout", s.auth(s.handleLogout))

	m.Handle("POST /api/uploads", s.auth(s.handleUpload))
	m.Handle("POST /api/invoices", s.auth(s.handleCreateInvoice))
	m.Handle("GET /api/invoices", s.auth(s.handleListInvoices))
	m.Handle("GET /api/invoices/{id}", s.auth(s.handleGetInvoice))
	m.Handle("PATCH /api/invoices/{id}", s.auth(s.handleUpdateInvoice))
	m.Handle("DELETE /api/invoices/{id}", s.auth(s.handleDeleteInvoice))
	m.Handle("GET /api/files/{id}", s.auth(s.handleFile))
	m.Handle("GET /api/files/{id}/thumb", s.auth(s.handleThumb))
	m.Handle("GET /api/stats", s.auth(s.handleStats))
	m.Handle("GET /api/export", s.auth(s.handleExport))

	// 管理员
	m.Handle("GET /api/users", s.admin(s.handleListUsers))
	m.Handle("POST /api/users", s.admin(s.handleCreateUser))
	m.Handle("PATCH /api/users/{id}", s.admin(s.handleUpdateUser))
	m.Handle("POST /api/users/{id}/password", s.admin(s.handleResetPassword))
	m.Handle("DELETE /api/users/{id}", s.admin(s.handleDeleteUser))
	m.Handle("GET /api/settings", s.auth(s.handleGetSettings))
	m.Handle("PUT /api/settings", s.admin(s.handlePutSettings))
	m.Handle("GET /api/admin/audit", s.admin(s.handleAudit))
	m.Handle("GET /api/admin/storage", s.admin(s.handleStorage))
	m.Handle("POST /api/admin/cleanup", s.admin(s.handleCleanup))
	m.Handle("GET /api/admin/backup", s.admin(s.handleBackup))

	// 前端静态资源
	m.HandleFunc("/", s.handleStatic)
}

// ---------------------------------------------------------------- 中间件
func (s *Server) auth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "请先登录", "unauthenticated")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

func (s *Server) admin(next http.HandlerFunc) http.Handler {
	return s.auth(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "admin" {
			writeErr(w, http.StatusForbidden, "需要管理员权限", "forbidden")
			return
		}
		next(w, r)
	})
}

func (s *Server) currentUser(r *http.Request) (*auth.User, error) {
	var token string
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		token = strings.TrimPrefix(h, "Bearer ")
	}
	if token == "" {
		if c, err := r.Cookie("ticketbox_sid"); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		// 浏览器直接下载附件时无法带请求头，允许用查询参数传 token
		if q := r.URL.Query().Get("token"); q != "" {
			token = q
		}
	}
	if token == "" {
		return nil, http.ErrNoCookie
	}
	claims, err := auth.ParseToken(token, s.cfg.Secret)
	if err != nil {
		return nil, err
	}
	var active bool
	if err := store.DB.QueryRow("SELECT active FROM users WHERE id=?", claims.UserID).Scan(&active); err != nil || !active {
		return nil, http.ErrNoCookie
	}
	return &auth.User{
		ID: claims.UserID, Username: claims.Username,
		DisplayName: claims.DisplayName, Role: claims.Role,
	}, nil
}

func currentUser(r *http.Request) *auth.User {
	if u, ok := r.Context().Value(userKey).(*auth.User); ok && u != nil {
		return u
	}
	return &auth.User{}
}

func (s *Server) withLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") || s.cfg.Debug {
			log.Printf("%-6s %-42s %3d %s", r.Method, r.URL.RequestURI(), rec.status,
				time.Since(start).Round(time.Millisecond))
		}
	})
}

func (s *Server) withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic: %v", v)
				writeErr(w, http.StatusInternalServerError, "服务器内部错误", "internal")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// ---------------------------------------------------------------- 响应
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeErr(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, map[string]any{"error": msg, "code": code})
}

func errf(w http.ResponseWriter, status int, code, msg string) {
	writeErr(w, status, msg, code)
}

// ---------------------------------------------------------------- 通用解析
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20))
	dec.UseNumber()
	return dec.Decode(dst)
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.Split(xff, ",")[0]
	}
	host := r.RemoteAddr
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return strings.Trim(host, "[]")
}

// ---------------------------------------------------------------- 静态资源
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "接口不存在", "not_found")
		return
	}
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." || clean == "" {
		clean = "index.html"
	}
	full := filepath.Join(s.cfg.WebDir, clean)
	root, _ := filepath.Abs(s.cfg.WebDir)
	abs, _ := filepath.Abs(full)
	if !strings.HasPrefix(abs, root) {
		http.NotFound(w, r)
		return
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		if filepath.Base(abs) == "index.html" {
			w.Header().Set("Cache-Control", "no-store, must-revalidate")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.ServeFile(w, r, abs)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err == nil {
		// 入口页禁止缓存，避免浏览器拿着旧的 index.html 继续加载旧版前端
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		http.ServeFile(w, r, index)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>票管家</title>
<body style="font-family:system-ui;padding:40px;line-height:1.8">
<h2>前端尚未构建</h2>
<p>请先在 <code>web/</code> 目录执行 <code>npm install &amp;&amp; npm run build</code>，或开发时使用 <code>npm run dev</code>（默认 http://127.0.0.1:5173）。</p>
</body>`))
}
