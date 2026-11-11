package api

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"invoice/server/internal/auth"
	"invoice/server/internal/model"
	"invoice/server/internal/store"
)

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,32}$`)

type loginAttempt struct {
	count int
	first time.Time
}

var (
	attemptMu sync.Mutex
	attempts  = map[string]loginAttempt{}
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": store.TimeText(time.Now())})
}

func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	var count int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	payload := map[string]any{
		"needs_setup": count == 0,
		"user":        nil,
		"settings":    publicSettings(),
		"app":         map[string]string{"name": "票管家", "version": "1.1.0"},
	}
	if u, err := s.currentUser(r); err == nil {
		payload["user"] = s.loadUserPublic(u.ID)
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var count int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		errf(w, http.StatusBadRequest, "already_setup", "系统已初始化，请直接登录")
		return
	}
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		OrgName     string `json:"org_name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	username := strings.TrimSpace(body.Username)
	if !usernameRe.MatchString(username) {
		errf(w, http.StatusBadRequest, "bad_username", "用户名需为 3-32 位字母、数字或 _ . -")
		return
	}
	if len(body.Password) < 6 {
		errf(w, http.StatusBadRequest, "bad_password", "密码至少 6 位")
		return
	}
	display := strings.TrimSpace(body.DisplayName)
	if display == "" {
		display = username
	}
	res, err := store.DB.Exec(
		"INSERT INTO users(username, display_name, password_hash, role, active, created_at) VALUES(?,?,?,'admin',1,?)",
		username, display, auth.HashPassword(body.Password), store.TimeText(time.Now()))
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "创建管理员失败: "+err.Error())
		return
	}
	id, _ := res.LastInsertId()
	if org := strings.TrimSpace(body.OrgName); org != "" {
		_ = store.SetSettings(map[string]string{"org_name": org})
	}
	store.Audit(&id, display, "setup", "system", "初始化管理员账号", clientIP(r))
	s.issueAndRespond(w, id)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	username := strings.TrimSpace(body.Username)
	key := clientIP(r) + ":" + strings.ToLower(username)
	if blocked(key) {
		errf(w, http.StatusTooManyRequests, "rate_limited", "尝试次数过多，请 5 分钟后再试")
		return
	}

	var id int64
	var hash, display, role string
	var active bool
	err := store.DB.QueryRow("SELECT id, password_hash, display_name, role, active FROM users WHERE username=?",
		username).Scan(&id, &hash, &display, &role, &active)
	if err != nil || !auth.VerifyPassword(body.Password, hash) {
		recordFail(key)
		errf(w, http.StatusUnauthorized, "bad_credentials", "用户名或密码错误")
		return
	}
	if !active {
		errf(w, http.StatusForbidden, "inactive", "账号已停用，请联系管理员")
		return
	}
	clearFails(key)
	_, _ = store.DB.Exec("UPDATE users SET last_login_at=? WHERE id=?", store.TimeText(time.Now()), id)
	store.Audit(&id, display, "login", "system", "", clientIP(r))
	s.issueAndRespond(w, id)
}

func (s *Server) issueAndRespond(w http.ResponseWriter, userID int64) {
	u := s.loadUserPublic(userID)
	if u == nil {
		errf(w, http.StatusInternalServerError, "db_error", "读取账号失败")
		return
	}
	token, err := auth.NewToken(auth.User{ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Role: u.Role},
		s.cfg.Secret, 7*24*time.Hour)
	if err != nil {
		errf(w, http.StatusInternalServerError, "token_error", "签发登录凭证失败")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "ticketbox_sid", Value: token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"token":    token,
		"user":     u,
		"settings": publicSettings(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "ticketbox_sid", Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":     s.loadUserPublic(u.ID),
		"settings": publicSettings(),
	})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	var hash string
	if err := store.DB.QueryRow("SELECT password_hash FROM users WHERE id=?", u.ID).Scan(&hash); err != nil {
		errf(w, http.StatusNotFound, "not_found", "账号不存在")
		return
	}
	if !auth.VerifyPassword(body.OldPassword, hash) {
		errf(w, http.StatusBadRequest, "bad_password", "原密码不正确")
		return
	}
	if len(body.NewPassword) < 6 {
		errf(w, http.StatusBadRequest, "bad_password", "新密码至少 6 位")
		return
	}
	_, _ = store.DB.Exec("UPDATE users SET password_hash=? WHERE id=?", auth.HashPassword(body.NewPassword), u.ID)
	store.Audit(&u.ID, u.DisplayName, "change_password", "user", "", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- 账号读取
func (s *Server) loadUserPublic(id int64) *model.User {
	var u model.User
	var created time.Time
	var last sql.NullTime
	err := store.DB.QueryRow("SELECT id, username, display_name, role, dept, active, created_at, last_login_at "+
		"FROM users WHERE id=?", id).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Dept, &u.Active, &created, &last)
	if err != nil {
		return nil
	}
	u.CreatedAtText = store.TimeText(created)
	if last.Valid {
		u.LastLoginAtText = store.TimeText(last.Time)
	}
	return &u
}

func publicSettings() map[string]any {
	s := store.GetSettings()
	list := []string{}
	for _, c := range strings.Split(s["categories"], ",") {
		if c = strings.TrimSpace(c); c != "" {
			list = append(list, c)
		}
	}
	return map[string]any{
		"org_name":       s["org_name"],
		"categories":     list,
		"currency":       s["currency"],
		"max_upload_mb":  s["max_upload_mb"],
		"allowed_exts":   s["allowed_exts"],
		"retention_mode": s["retention_mode"],
		"retention_days": s["retention_days"],
		"retention_scope": s["retention_scope"],
		"ocr_engine":     s["ocr_engine"],
	}
}

// ---------------------------------------------------------------- 限流
func blocked(key string) bool {
	attemptMu.Lock()
	defer attemptMu.Unlock()
	a, ok := attempts[key]
	if !ok {
		return false
	}
	if time.Since(a.first) > 5*time.Minute {
		delete(attempts, key)
		return false
	}
	return a.count >= 10
}

func recordFail(key string) {
	attemptMu.Lock()
	defer attemptMu.Unlock()
	a := attempts[key]
	if time.Since(a.first) > 5*time.Minute || a.first.IsZero() {
		a = loginAttempt{count: 0, first: time.Now()}
	}
	a.count++
	attempts[key] = a
}

func clearFails(key string) {
	attemptMu.Lock()
	defer attemptMu.Unlock()
	delete(attempts, key)
}
