package api

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"invoice/server/internal/auth"
	"invoice/server/internal/exporter"
	"invoice/server/internal/model"
	"invoice/server/internal/ocr"
	"invoice/server/internal/storage"
	"invoice/server/internal/store"
)

// ---------------------------------------------------------------- 成员
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := store.DB.Query("SELECT id, username, display_name, role, dept, active, created_at, last_login_at FROM users ORDER BY (role='admin') DESC, id")
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "查询成员失败")
		return
	}
	defer rows.Close()
	type stat struct {
		count  int64
		amount float64
	}
	stats := map[int64]stat{}
	if sr, err := store.DB.Query("SELECT user_id, COUNT(*), COALESCE(SUM(amount),0) FROM invoices WHERE deleted=0 GROUP BY user_id"); err == nil {
		for sr.Next() {
			var uid int64
			var c int64
			var a float64
			if sr.Scan(&uid, &c, &a) == nil {
				stats[uid] = stat{count: c, amount: a}
			}
		}
		sr.Close()
	}
	users := []model.User{}
	for rows.Next() {
		var u model.User
		var created time.Time
		var last sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.Dept, &u.Active, &created, &last); err != nil {
			continue
		}
		u.CreatedAtText = store.TimeText(created)
		if last.Valid {
			u.LastLoginAtText = store.TimeText(last.Time)
		}
		if st, ok := stats[u.ID]; ok {
			u.InvoiceCount = st.count
			u.InvoiceAmount = st.amount
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
		Role        string `json:"role"`
		Dept        string `json:"dept"`
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
	role := "member"
	if body.Role == "admin" {
		role = "admin"
	}
	display := strings.TrimSpace(body.DisplayName)
	if display == "" {
		display = username
	}
	res, err := store.DB.Exec("INSERT INTO users(username, display_name, password_hash, role, active, dept, created_at) VALUES(?,?,?,?,1,?,?)",
		username, display, auth.HashPassword(body.Password), role, clampString(body.Dept, 40), store.TimeText(time.Now()))
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			errf(w, http.StatusBadRequest, "duplicate", "用户名已存在")
			return
		}
		errf(w, http.StatusInternalServerError, "db_error", "创建成员失败")
		return
	}
	id, _ := res.LastInsertId()
	u := currentUser(r)
	store.Audit(&u.ID, u.DisplayName, "create_user", fmt.Sprintf("user:%d", id), username, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"user": s.loadUserPublic(id)})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	sets := []string{}
	args := []any{}
	if v, ok := body["display_name"].(string); ok {
		sets = append(sets, "display_name=?")
		args = append(args, clampString(v, 32))
	}
	if v, ok := body["dept"].(string); ok {
		sets = append(sets, "dept=?")
		args = append(args, clampString(v, 40))
	}
	if v, ok := body["active"].(bool); ok {
		sets = append(sets, "active=?")
		args = append(args, v)
	}
	if v, ok := body["role"].(string); ok && (v == "admin" || v == "member") {
		if v == "member" && id == currentUser(r).ID {
			errf(w, http.StatusBadRequest, "self_demote", "不能降级自己")
			return
		}
		sets = append(sets, "role=?")
		args = append(args, v)
	}
	if len(sets) == 0 {
		errf(w, http.StatusBadRequest, "no_change", "没有需要更新的内容")
		return
	}
	args = append(args, id)
	if _, err := store.DB.Exec("UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id=?", args...); err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "更新失败")
		return
	}
	u := currentUser(r)
	store.Audit(&u.ID, u.DisplayName, "update_user", fmt.Sprintf("user:%d", id), fmt.Sprintf("%v", body), clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"user": s.loadUserPublic(id)})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil || len(body.Password) < 6 {
		errf(w, http.StatusBadRequest, "bad_password", "密码至少 6 位")
		return
	}
	if _, err := store.DB.Exec("UPDATE users SET password_hash=? WHERE id=?", auth.HashPassword(body.Password), id); err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "重置失败")
		return
	}
	u := currentUser(r)
	store.Audit(&u.ID, u.DisplayName, "reset_password", fmt.Sprintf("user:%d", id), "", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if id == currentUser(r).ID {
		errf(w, http.StatusBadRequest, "self_delete", "不能删除自己")
		return
	}
	var count int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM invoices WHERE user_id=? AND deleted=0", id).Scan(&count)
	if count > 0 {
		errf(w, http.StatusBadRequest, "has_invoices", fmt.Sprintf("该成员名下还有 %d 张票据，请先处理", count))
		return
	}
	u := currentUser(r)
	_, _ = store.DB.Exec("DELETE FROM users WHERE id=?", id)
	store.Audit(&u.ID, u.DisplayName, "delete_user", fmt.Sprintf("user:%d", id), "", clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- 设置
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if currentUser(r).Role != "admin" {
		writeJSON(w, http.StatusOK, map[string]any{"settings": publicSettings()})
		return
	}
	all := store.GetSettings()
	out := map[string]any{}
	for k, v := range all {
		out[k] = v
	}
	list := []string{}
	for _, c := range strings.Split(all["categories"], ",") {
		if c = strings.TrimSpace(c); c != "" {
			list = append(list, c)
		}
	}
	out["categories"] = list
	engine := ocr.CurrentEngine()
	out["ocr_available"] = engine.Available
	out["ocr_engine_name"] = engine.Name
	out["ocr_engine_label"] = engine.Label
	out["ocr_detail"] = engine.Detail
	out["ocr_platform"] = engine.Platform
	writeJSON(w, http.StatusOK, map[string]any{"settings": out})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	allowed := map[string]int{"org_name": 40, "categories": 400, "max_upload_mb": 4,
		"allowed_exts": 120, "retention_mode": 16, "retention_days": 6,
		"retention_scope": 16, "ocr_engine": 16}
	updates := map[string]string{}
	for k, limit := range allowed {
		raw, ok := body[k]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case string:
			updates[k] = clampString(v, limit)
		case []any:
			parts := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					parts = append(parts, clampString(s, 40))
				}
			}
			updates[k] = strings.Join(parts, ",")
		case float64:
			updates[k] = strconv.Itoa(int(v))
		}
	}
	if v, ok := updates["retention_mode"]; ok && v != "permanent" && v != "auto" {
		errf(w, http.StatusBadRequest, "bad_setting", "保留策略不合法")
		return
	}
	if v, ok := updates["retention_scope"]; ok && v != "files" && v != "all" {
		errf(w, http.StatusBadRequest, "bad_setting", "清理范围不合法")
		return
	}
	if len(updates) == 0 {
		errf(w, http.StatusBadRequest, "no_change", "没有需要更新的内容")
		return
	}
	if err := store.SetSettings(updates); err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "保存失败")
		return
	}
	u := currentUser(r)
	store.Audit(&u.ID, u.DisplayName, "update_settings", "settings", fmt.Sprintf("%v", updates), clientIP(r))
	s.handleGetSettings(w, r)
}

// ---------------------------------------------------------------- 运维
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 100)
	if limit > 500 {
		limit = 500
	}
	rows, err := store.DB.Query("SELECT id, user_id, actor_name, action, target, detail, ip, created_at "+
		"FROM audit_logs ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "查询日志失败")
		return
	}
	defer rows.Close()
	logs := []map[string]any{}
	for rows.Next() {
		var id int64
		var uid sql.NullInt64
		var actor, action, target, detail, ip string
		var created time.Time
		if rows.Scan(&id, &uid, &actor, &action, &target, &detail, &ip, &created) == nil {
			logs = append(logs, map[string]any{
				"id": id, "actor_name": actor, "action": action, "target": target,
				"detail": detail, "ip": ip, "created_at": store.TimeText(created),
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	files, bytesUsed := storage.Usage(s.cfg)
	var invoices int
	_ = store.DB.QueryRow("SELECT COUNT(*) FROM invoices WHERE deleted=0").Scan(&invoices)
	settings := store.GetSettings()
	writeJSON(w, http.StatusOK, map[string]any{
		"files": files, "bytes": bytesUsed, "invoices": invoices,
		"settings": map[string]string{
			"retention_mode":  settings["retention_mode"],
			"retention_days":  settings["retention_days"],
			"retention_scope": settings["retention_scope"],
		},
		"data_dir": s.cfg.DataDir,
	})
}

func (s *Server) handleCleanup(w http.ResponseWriter, r *http.Request) {
	result := s.runRetention()
	storage.CleanupTemp(s.cfg, 12*time.Hour)
	store.Audit(nil, "system", "retention_cleanup", "system", fmt.Sprintf("%v", result), clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func (s *Server) runRetention() map[string]any {
	settings := store.GetSettings()
	if settings["retention_mode"] != "auto" {
		return map[string]any{"skipped": true, "reason": "permanent"}
	}
	days, err := strconv.Atoi(settings["retention_days"])
	if err != nil || days < 1 {
		days = 365
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := store.DB.Query("SELECT id FROM invoices WHERE deleted=0 AND DATE(created_at) < ?", cutoff)
	if err != nil {
		return map[string]any{"skipped": true, "reason": err.Error()}
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	scope := settings["retention_scope"]
	removedFiles := 0
	for _, id := range ids {
		files := loadFiles(id)
		for _, f := range files {
			storage.Delete(s.cfg, f.StorageName)
			removedFiles++
		}
		if scope == "all" {
			_, _ = store.DB.Exec("DELETE FROM files WHERE invoice_id=?", id)
			_, _ = store.DB.Exec("DELETE FROM invoices WHERE id=?", id)
		} else {
			_, _ = store.DB.Exec("UPDATE files SET deleted=1 WHERE invoice_id=?", id)
		}
	}
	return map[string]any{"skipped": false, "expired": len(ids), "files": removedFiles,
		"cutoff": cutoff, "scope": scope, "today": time.Now().Format("2006-01-02")}
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	dump := map[string]any{}
	users := []map[string]any{}
	if rows, err := store.DB.Query("SELECT id, username, display_name, role, dept, active, created_at FROM users"); err == nil {
		for rows.Next() {
			var id int64
			var username, display, role, dept string
			var active bool
			var created time.Time
			if rows.Scan(&id, &username, &display, &role, &dept, &active, &created) == nil {
				users = append(users, map[string]any{"id": id, "username": username, "display_name": display,
					"role": role, "dept": dept, "active": active, "created_at": store.TimeText(created)})
			}
		}
		rows.Close()
	}
	dump["users"] = users
	dump["settings"] = store.GetSettings()
	invoices := []model.Invoice{}
	if rows, err := store.DB.Query(invoiceSelect + "WHERE i.deleted=0"); err == nil {
		for rows.Next() {
			if inv, err := scanInvoice(rows); err == nil {
				invoices = append(invoices, inv)
			}
		}
		rows.Close()
	}
	dump["invoices"] = invoices
	dump["exported_at"] = store.TimeText(time.Now())
	payload, _ := json.MarshalIndent(dump, "", "  ")
	if w, err := zw.Create("data.json"); err == nil {
		_, _ = w.Write(payload)
	}
	filesDir := filepath.Join(s.cfg.DataDir, "files")
	if entries, err := os.ReadDir(filesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(filesDir, e.Name()))
			if err != nil {
				continue
			}
			if w, err := zw.Create("files/" + e.Name()); err == nil {
				_, _ = w.Write(data)
			}
		}
	}
	_ = zw.Close()
	name := fmt.Sprintf("发票系统备份_%s.zip", time.Now().Format("20060102_1504"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEncode(name))
	_, _ = w.Write(buf.Bytes())
}

// ---------------------------------------------------------------- 导出
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	where, args := s.buildFilters(r)
	rows, err := store.DB.Query(invoiceSelect+"WHERE "+where+" ORDER BY i.created_at ASC, i.id ASC", args...)
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "查询失败")
		return
	}
	defer rows.Close()
	var invoices []model.Invoice
	for rows.Next() {
		if inv, err := scanInvoice(rows); err == nil {
			invoices = append(invoices, inv)
		}
	}
	if len(invoices) == 0 {
		errf(w, http.StatusBadRequest, "empty", "当前筛选条件下没有可导出的票据")
		return
	}
	fileMap := map[int64][]model.File{}
	for _, inv := range invoices {
		fileMap[inv.ID] = loadFiles(inv.ID)
	}
	rangeLabel := rangeLabelFor(r)
	meta := exporter.Meta{
		RangeLabel:  rangeLabel,
		StatusLabel: statusText(r.URL.Query().Get("status")),
		TypeFilter:  typeText(r.URL.Query().Get("type")),
	}
	data, name, err := exporter.BuildZip(s.cfg, invoices, fileMap, meta)
	if err != nil {
		errf(w, http.StatusInternalServerError, "export_error", "生成压缩包失败："+err.Error())
		return
	}
	u := currentUser(r)
	store.Audit(&u.ID, u.DisplayName, "export", "invoices",
		fmt.Sprintf("count=%d range=%s status=%s", len(invoices), rangeLabel, meta.StatusLabel), clientIP(r))

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+urlEncode(name))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func rangeLabelFor(r *http.Request) string {
	q := r.URL.Query()
	year, month := q.Get("year"), q.Get("month")
	if year != "" && month != "" {
		n, _ := strconv.Atoi(month)
		return fmt.Sprintf("%s年%02d月", year, n)
	}
	if year != "" {
		return year + "年"
	}
	if q.Get("date_from") != "" || q.Get("date_to") != "" {
		return fmt.Sprintf("%s 至 %s", orDefault(q.Get("date_from"), "最早"), orDefault(q.Get("date_to"), "今天"))
	}
	switch q.Get("status") {
	case "pending":
		return "未报销"
	case "reimbursed":
		return "已报销"
	}
	return "全部"
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func statusText(v string) string {
	switch v {
	case "pending":
		return "未报销"
	case "reimbursed":
		return "已报销"
	}
	return "全部"
}

func typeText(v string) string {
	switch v {
	case "procurement":
		return "采购报销"
	case "travel":
		return "出行报销"
	}
	return "全部"
}
