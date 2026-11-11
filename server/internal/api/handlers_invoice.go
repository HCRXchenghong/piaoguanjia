package api

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"invoice/server/internal/model"
	"invoice/server/internal/ocr"
	"invoice/server/internal/storage"
	"invoice/server/internal/store"
)

const invoiceSelect = `SELECT i.id, i.user_id, u.display_name, i.type, i.status, i.title, i.category, i.spec, i.unit,
i.link, i.qty, i.unit_price, i.shipping, i.amount, i.travel_time, i.transport, i.route, i.traveler,
i.note, i.invoice_no, i.invoice_code, i.invoice_date, i.seller, i.buyer, i.tax, i.total,
i.invoice_kind, i.ocr_engine, i.ocr_confidence, i.created_at, i.updated_at, i.reimbursed_at
FROM invoices i JOIN users u ON u.id = i.user_id `

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInvoice(s rowScanner) (model.Invoice, error) {
	var inv model.Invoice
	var qty, unitPrice, shipping, tax, total sql.NullFloat64
	var created, updated time.Time
	var reimbursed sql.NullTime
	err := s.Scan(&inv.ID, &inv.UserID, &inv.DisplayName, &inv.Type, &inv.Status, &inv.Title, &inv.Category,
		&inv.Spec, &inv.Unit, &inv.Link, &qty, &unitPrice, &shipping, &inv.Amount, &inv.TravelTime, &inv.Transport,
		&inv.Route, &inv.Traveler, &inv.Note, &inv.InvoiceNo, &inv.InvoiceCode, &inv.InvoiceDate,
		&inv.Seller, &inv.Buyer, &tax, &total, &inv.InvoiceKind, &inv.OCREngine, &inv.OCRConfidence,
		&created, &updated, &reimbursed)
	if err != nil {
		return inv, err
	}
	if qty.Valid {
		inv.Qty = &qty.Float64
	}
	if unitPrice.Valid {
		inv.UnitPrice = &unitPrice.Float64
	}
	if shipping.Valid {
		inv.Shipping = &shipping.Float64
	}
	if tax.Valid {
		inv.Tax = &tax.Float64
	}
	if total.Valid {
		inv.Total = &total.Float64
	}
	inv.CreatedAtText = store.TimeText(created)
	inv.UpdatedAtText = store.TimeText(updated)
	if reimbursed.Valid {
		inv.ReimbursedText = store.TimeText(reimbursed.Time)
	}
	inv.TypeLabel = map[string]string{"procurement": "采购报销", "travel": "出行报销"}[inv.Type]
	inv.StatusLabel = map[string]string{"pending": "未报销", "reimbursed": "已报销"}[inv.Status]
	inv.Attachments = nil
	return inv, nil
}

func loadFiles(invoiceID int64) []model.File {
	rows, err := store.DB.Query("SELECT id, invoice_id, storage_name, original_name, mime, size, sha256, created_at "+
		"FROM files WHERE invoice_id=? AND deleted=0 ORDER BY id", invoiceID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []model.File
	for rows.Next() {
		var f model.File
		var created time.Time
		if err := rows.Scan(&f.ID, &f.InvoiceID, &f.StorageName, &f.OriginalName, &f.Mime, &f.Size, &f.SHA256, &created); err == nil {
			f.Ext = strings.TrimPrefix(filepath.Ext(f.StorageName), ".")
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------- 上传识别
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		errf(w, http.StatusBadRequest, "bad_form", "上传格式不正确："+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		errf(w, http.StatusBadRequest, "no_file", "没有收到文件")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		errf(w, http.StatusBadRequest, "read_error", "读取文件失败")
		return
	}
	saved, err := storage.Save(s.cfg, data, header.Filename)
	if err != nil {
		errf(w, http.StatusBadRequest, "bad_file", err.Error())
		return
	}

	settings := store.GetSettings()
	cfgMap := map[string]string{}
	for k, v := range settings {
		cfgMap[k] = v
	}
	started := time.Now()
	var parsed *model.ParseResult
	if settings["ocr_engine"] == "none" {
		parsed = &model.ParseResult{
			Kind: "unknown", KindLabel: "已关闭识别", Engine: "none",
			Fields: map[string]model.Field{}, Items: []model.Item{},
			Suggest: map[string]any{"type": "procurement"}, Warnings: []string{"已关闭自动识别，请手动填写"},
		}
	} else {
		parsed = ocr.Analyze(s.cfg, data, header.Filename, cfgMap)
	}

	token := randomToken(24)
	tmpPath := filepath.Join(s.cfg.DataDir, "tmp", "upload_"+token+saved.Ext)
	if err := os.Rename(saved.Path, tmpPath); err != nil {
		if copyErr := copyFile(saved.Path, tmpPath); copyErr != nil {
			errf(w, http.StatusInternalServerError, "storage_error", "临时文件保存失败")
			return
		}
		_ = os.Remove(saved.Path)
	}

	parseJSON, _ := json.Marshal(parsed)
	_, err = store.DB.Exec("INSERT INTO pending_uploads(token, user_id, filename, path, mime, size, sha256, parse_json, created_at) "+
		"VALUES(?,?,?,?,?,?,?,?,?)", token, currentUser(r).ID, saved.OriginalName, tmpPath, saved.Mime,
		saved.Size, saved.SHA256, string(parseJSON), store.TimeText(time.Now()))
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "保存上传记录失败")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"file": map[string]any{
			"name": saved.OriginalName, "mime": saved.Mime, "size": saved.Size, "ext": saved.Ext,
		},
		"parse":      parsed,
		"elapsed_ms": time.Since(started).Milliseconds(),
	})
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// ---------------------------------------------------------------- 创建票据
type createPayload struct {
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	Category   string   `json:"category"`
	Spec       string   `json:"spec"`
	Unit       string   `json:"unit"`
	Link       string   `json:"link"`
	Qty        *float64 `json:"qty"`
	UnitPrice  *float64 `json:"unit_price"`
	Shipping   *float64 `json:"shipping"`
	Amount     *float64 `json:"amount"`
	TravelTime string   `json:"travel_time"`
	Transport  string   `json:"transport"`
	Route      string   `json:"route"`
	Traveler   string   `json:"traveler"`
	Note       string   `json:"note"`
	Meta       struct {
		InvoiceNo   string   `json:"invoice_no"`
		InvoiceDate string   `json:"invoice_date"`
		Seller      string   `json:"seller"`
		Buyer       string   `json:"buyer"`
		Total       *float64 `json:"total"`
		Tax         *float64 `json:"tax"`
		InvoiceKind string   `json:"invoice_kind"`
	} `json:"meta"`
	OCREngine     string  `json:"ocr_engine"`
	OCRConfidence float64 `json:"ocr_confidence"`
}

func (s *Server) handleCreateInvoice(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Payload createPayload `json:"payload"`
		Uploads []string      `json:"uploads"`
	}
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	p := body.Payload
	if p.Type != "procurement" && p.Type != "travel" {
		errf(w, http.StatusBadRequest, "bad_type", "报销类型不正确")
		return
	}
	title := clampString(p.Title, 120)
	if title == "" {
		errf(w, http.StatusBadRequest, "bad_title", "请填写名称或事由")
		return
	}
	if len(body.Uploads) == 0 {
		errf(w, http.StatusBadRequest, "no_upload", "请至少上传一张发票文件")
		return
	}
	if len(body.Uploads) > 8 {
		errf(w, http.StatusBadRequest, "too_many", "单次最多 8 个附件")
		return
	}

	user := currentUser(r)
	type pending struct {
		token, path, filename, mime string
		size                        int64
		sha                         string
		parseJSON                   string
	}
	var uploads []pending
	for _, token := range body.Uploads {
		var up pending
		up.token = token
		err := store.DB.QueryRow("SELECT path, filename, mime, size, sha256, parse_json FROM pending_uploads WHERE token=? AND user_id=?",
			token, user.ID).Scan(&up.path, &up.filename, &up.mime, &up.size, &up.sha, &up.parseJSON)
		if err != nil {
			errf(w, http.StatusBadRequest, "upload_expired", "附件已失效，请重新上传")
			return
		}
		if _, err := os.Stat(up.path); err != nil {
			errf(w, http.StatusBadRequest, "upload_expired", "附件已失效，请重新上传")
			return
		}
		uploads = append(uploads, up)
	}

	amount := 0.0
	if p.Amount != nil {
		amount = *p.Amount
	} else if p.Qty != nil && p.UnitPrice != nil {
		amount = round2(*p.Qty**p.UnitPrice + floatValue(p.Shipping))
	}
	if amount < 0 {
		errf(w, http.StatusBadRequest, "bad_amount", "金额不能为负数")
		return
	}
	shipping := floatValue(p.Shipping)

	// 单位不在表单里编辑：优先取提交值，否则从上传时保存的识别结果里取第一行明细的单位。
	unit := clampString(p.Unit, 16)
	if unit == "" {
		for _, up := range uploads {
			var parsed model.ParseResult
			if json.Unmarshal([]byte(up.parseJSON), &parsed) == nil && len(parsed.Items) > 0 {
				unit = clampString(parsed.Items[0].Unit, 16)
				if unit != "" {
					break
				}
			}
		}
	}

	res, err := store.DB.Exec(`INSERT INTO invoices(user_id, type, status, title, category, spec, unit, link, qty,
		unit_price, shipping, amount, travel_time, transport, route, traveler, note, invoice_no, invoice_code,
		invoice_date, seller, buyer, tax, total, invoice_kind, ocr_engine, ocr_confidence, fields_json,
		source_text, created_at, updated_at)
		VALUES(?,?,'pending',?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		user.ID, p.Type, title, clampString(p.Category, 40), clampString(p.Spec, 120), unit, clampString(p.Link, 500),
		p.Qty, p.UnitPrice, shipping, amount, clampString(p.TravelTime, 40), clampString(p.Transport, 20),
		clampString(p.Route, 120), clampString(p.Traveler, 40), clampString(p.Note, 2000),
		clampString(p.Meta.InvoiceNo, 40), "", clampString(p.Meta.InvoiceDate, 20), clampString(p.Meta.Seller, 200),
		clampString(p.Meta.Buyer, 200), p.Meta.Tax, p.Meta.Total, clampString(p.Meta.InvoiceKind, 64),
		clampString(p.OCREngine, 32), p.OCRConfidence, "{}", "", store.TimeText(time.Now()), store.TimeText(time.Now()))
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "保存票据失败："+err.Error())
		return
	}
	invoiceID, _ := res.LastInsertId()

	for _, up := range uploads {
		ext := filepath.Ext(up.path)
		moved, name, err := storage.MoveInto(s.cfg, up.path, ext)
		if err != nil {
			continue
		}
		_, _ = store.DB.Exec("INSERT INTO files(invoice_id, storage_name, original_name, mime, size, sha256, kind, created_at) "+
			"VALUES(?,?,?,?,?,?,'original',?)", invoiceID, name, up.filename, up.mime, up.size, moved.SHA256,
			store.TimeText(time.Now()))
		_, _ = store.DB.Exec("DELETE FROM pending_uploads WHERE token=?", up.token)
	}

	store.Audit(&user.ID, user.DisplayName, "create_invoice", fmt.Sprintf("invoice:%d", invoiceID),
		fmt.Sprintf("title=%s amount=%.2f type=%s", title, amount, p.Type), clientIP(r))

	inv, _ := s.getInvoiceByID(invoiceID)
	writeJSON(w, http.StatusOK, map[string]any{"invoice": inv})
}

func (s *Server) getInvoiceByID(id int64) (*model.Invoice, error) {
	row := store.DB.QueryRow(invoiceSelect+"WHERE i.id=?", id)
	inv, err := scanInvoice(row)
	if err != nil {
		return nil, err
	}
	inv.Files = loadFiles(id)
	return &inv, nil
}

// ---------------------------------------------------------------- 列表
func (s *Server) buildFilters(r *http.Request) (string, []any) {
	where := []string{"i.deleted=0"}
	var args []any
	user := currentUser(r)
	scope := r.URL.Query().Get("scope")
	if user.Role != "admin" || scope == "mine" {
		where = append(where, "i.user_id=?")
		args = append(args, user.ID)
	} else if uid := r.URL.Query().Get("user_id"); uid != "" {
		if v, err := strconv.ParseInt(uid, 10, 64); err == nil {
			where = append(where, "i.user_id=?")
			args = append(args, v)
		}
	}
	q := r.URL.Query()
	if st := q.Get("status"); st == "pending" || st == "reimbursed" {
		where = append(where, "i.status=?")
		args = append(args, st)
	}
	if tp := q.Get("type"); tp == "procurement" || tp == "travel" {
		where = append(where, "i.type=?")
		args = append(args, tp)
	}
	if cat := q.Get("category"); cat != "" {
		where = append(where, "i.category=?")
		args = append(args, cat)
	}
	dateExpr := "COALESCE(NULLIF(i.invoice_date,''), DATE_FORMAT(i.created_at,'%Y-%m-%d'))"
	if y := q.Get("year"); y != "" {
		where = append(where, "SUBSTRING("+dateExpr+",1,4)=?")
		args = append(args, y)
	}
	if m := q.Get("month"); m != "" {
		if n, err := strconv.Atoi(m); err == nil {
			where = append(where, "SUBSTRING("+dateExpr+",6,2)=?")
			args = append(args, fmt.Sprintf("%02d", n))
		}
	}
	if v := q.Get("date_from"); v != "" {
		where = append(where, dateExpr+">=?")
		args = append(args, v)
	}
	if v := q.Get("date_to"); v != "" {
		where = append(where, dateExpr+"<=?")
		args = append(args, v)
	}
	if kw := strings.TrimSpace(q.Get("q")); kw != "" {
		like := "%" + kw + "%"
		where = append(where, "(i.title LIKE ? OR i.category LIKE ? OR i.seller LIKE ? OR i.note LIKE ? OR i.invoice_no LIKE ? OR i.route LIKE ? OR i.traveler LIKE ?)")
		for i := 0; i < 7; i++ {
			args = append(args, like)
		}
	}
	return strings.Join(where, " AND "), args
}

func (s *Server) handleListInvoices(w http.ResponseWriter, r *http.Request) {
	where, args := s.buildFilters(r)
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	size := queryInt(r, "page_size", 20)
	if size < 1 || size > 200 {
		size = 20
	}
	var total int
	var sumAmount float64
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM invoices i WHERE "+where, args...).Scan(&total); err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "查询失败")
		return
	}
	_ = store.DB.QueryRow("SELECT COALESCE(SUM(i.amount),0) FROM invoices i WHERE "+where, args...).Scan(&sumAmount)

	query := invoiceSelect + "WHERE " + where + " ORDER BY i.created_at DESC, i.id DESC LIMIT ? OFFSET ?"
	rows, err := store.DB.Query(query, append(args, size, (page-1)*size)...)
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "查询失败："+err.Error())
		return
	}
	defer rows.Close()
	items := []model.Invoice{}
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			continue
		}
		inv.Files = loadFiles(inv.ID)
		items = append(items, inv)
	}
	pages := (total + size - 1) / size
	if pages < 1 {
		pages = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "sum_amount": round2(sumAmount),
		"page": page, "page_size": size, "pages": pages,
	})
}

func (s *Server) handleGetInvoice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	inv, err := s.getInvoiceByID(id)
	if err != nil {
		errf(w, http.StatusNotFound, "not_found", "票据不存在")
		return
	}
	if !s.canAccess(r, inv.UserID) {
		errf(w, http.StatusForbidden, "forbidden", "无权查看该票据")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"invoice": inv})
}

func (s *Server) canAccess(r *http.Request, ownerID int64) bool {
	u := currentUser(r)
	return u.Role == "admin" || u.ID == ownerID
}

func (s *Server) handleUpdateInvoice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var ownerID int64
	if err := store.DB.QueryRow("SELECT user_id FROM invoices WHERE id=? AND deleted=0", id).Scan(&ownerID); err != nil {
		errf(w, http.StatusNotFound, "not_found", "票据不存在")
		return
	}
	if !s.canAccess(r, ownerID) {
		errf(w, http.StatusForbidden, "forbidden", "无权修改该票据")
		return
	}
	var body map[string]any
	if err := decodeJSON(r, &body); err != nil {
		errf(w, http.StatusBadRequest, "bad_json", "请求体格式不正确")
		return
	}
	sets := []string{}
	args := []any{}
	user := currentUser(r)

	if v, ok := body["status"].(string); ok {
		if v != "pending" && v != "reimbursed" {
			errf(w, http.StatusBadRequest, "bad_status", "状态不合法")
			return
		}
		sets = append(sets, "status=?")
		args = append(args, v)
		if v == "reimbursed" {
			sets = append(sets, "reimbursed_at=?", "reimbursed_by=?")
			args = append(args, store.TimeText(time.Now()), user.ID)
		} else {
			sets = append(sets, "reimbursed_at=NULL", "reimbursed_by=NULL")
		}
	}
	textFields := map[string]int{"title": 120, "category": 40, "spec": 120, "unit": 16, "link": 500,
		"travel_time": 40, "transport": 20, "route": 120, "traveler": 40, "note": 2000}
	for k, limit := range textFields {
		if v, ok := body[k].(string); ok {
			sets = append(sets, k+"=?")
			args = append(args, clampString(v, limit))
		}
	}
	for _, k := range []string{"qty", "unit_price", "shipping", "amount"} {
		if v, ok := body[k]; ok {
			sets = append(sets, k+"=?")
			args = append(args, toNumber(v))
		}
	}
	if len(sets) == 0 {
		errf(w, http.StatusBadRequest, "no_change", "没有需要更新的内容")
		return
	}
	sets = append(sets, "updated_at=?")
	args = append(args, store.TimeText(time.Now()), id)
	if _, err := store.DB.Exec("UPDATE invoices SET "+strings.Join(sets, ", ")+" WHERE id=?", args...); err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "更新失败："+err.Error())
		return
	}
	store.Audit(&user.ID, user.DisplayName, "update_invoice", fmt.Sprintf("invoice:%d", id),
		fmt.Sprintf("%v", body), clientIP(r))
	inv, _ := s.getInvoiceByID(id)
	writeJSON(w, http.StatusOK, map[string]any{"invoice": inv})
}

func (s *Server) handleDeleteInvoice(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var ownerID int64
	var title string
	if err := store.DB.QueryRow("SELECT user_id, title FROM invoices WHERE id=? AND deleted=0", id).Scan(&ownerID, &title); err != nil {
		errf(w, http.StatusNotFound, "not_found", "票据不存在")
		return
	}
	if !s.canAccess(r, ownerID) {
		errf(w, http.StatusForbidden, "forbidden", "无权删除该票据")
		return
	}
	user := currentUser(r)
	_, _ = store.DB.Exec("UPDATE invoices SET deleted=1, deleted_at=? WHERE id=?", store.TimeText(time.Now()), id)
	store.Audit(&user.ID, user.DisplayName, "delete_invoice", fmt.Sprintf("invoice:%d", id), title, clientIP(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------- 附件
func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var storageName, originalName, mime string
	var ownerID int64
	err := store.DB.QueryRow(`SELECT f.storage_name, f.original_name, f.mime, i.user_id FROM files f
		JOIN invoices i ON i.id=f.invoice_id WHERE f.id=? AND f.deleted=0 AND i.deleted=0`, id).
		Scan(&storageName, &originalName, &mime, &ownerID)
	if err != nil {
		errf(w, http.StatusNotFound, "not_found", "文件不存在")
		return
	}
	if !s.canAccess(r, ownerID) {
		errf(w, http.StatusForbidden, "forbidden", "无权查看该文件")
		return
	}
	path := storage.Path(s.cfg, storageName)
	if _, err := os.Stat(path); err != nil {
		errf(w, http.StatusNotFound, "not_found", "文件已丢失")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", "inline; filename*=UTF-8''"+urlEncode(originalName))
	w.Header().Set("Cache-Control", "private, max-age=600")
	http.ServeFile(w, r, path)
}

func (s *Server) handleThumb(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var f model.File
	var ownerID int64
	err := store.DB.QueryRow(`SELECT f.id, f.invoice_id, f.storage_name, f.original_name, f.mime, f.size, i.user_id
		FROM files f JOIN invoices i ON i.id=f.invoice_id WHERE f.id=? AND f.deleted=0 AND i.deleted=0`, id).
		Scan(&f.ID, &f.InvoiceID, &f.StorageName, &f.OriginalName, &f.Mime, &f.Size, &ownerID)
	if err != nil {
		errf(w, http.StatusNotFound, "not_found", "文件不存在")
		return
	}
	if !s.canAccess(r, ownerID) {
		errf(w, http.StatusForbidden, "forbidden", "无权查看该文件")
		return
	}
	data, err := storage.Thumbnail(s.cfg, f)
	if err != nil {
		errf(w, http.StatusNotFound, "no_thumb", "无法生成预览图")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

// ---------------------------------------------------------------- 统计
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	where, args := s.buildFilters(r)
	var count int
	var amount, pendingAmt, doneAmt float64
	var pendingCnt, doneCnt int
	err := store.DB.QueryRow(fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(i.amount),0),
		COALESCE(SUM(CASE WHEN i.status='pending' THEN i.amount ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN i.status='reimbursed' THEN i.amount ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN i.status='pending' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN i.status='reimbursed' THEN 1 ELSE 0 END),0)
		FROM invoices i WHERE %s`, where), args...).
		Scan(&count, &amount, &pendingAmt, &doneAmt, &pendingCnt, &doneCnt)
	if err != nil {
		errf(w, http.StatusInternalServerError, "db_error", "统计失败："+err.Error())
		return
	}
	year := r.URL.Query().Get("year")
	if year == "" {
		year = time.Now().Format("2006")
	}
	dateExpr := "COALESCE(NULLIF(i.invoice_date,''), DATE_FORMAT(i.created_at,'%Y-%m-%d'))"
	monthRows, err := store.DB.Query(fmt.Sprintf(`SELECT SUBSTRING(%s,6,2) AS m, COUNT(*), COALESCE(SUM(i.amount),0)
		FROM invoices i WHERE %s AND SUBSTRING(%s,1,4)=? GROUP BY m ORDER BY m`, dateExpr, where, dateExpr),
		append(args, year)...)
	months := []map[string]any{}
	if err == nil {
		defer monthRows.Close()
		for monthRows.Next() {
			var m string
			var c int
			var a float64
			if monthRows.Scan(&m, &c, &a) == nil {
				mi, _ := strconv.Atoi(m)
				months = append(months, map[string]any{"month": mi, "count": c, "amount": round2(a)})
			}
		}
	}

	catRows, err := store.DB.Query(fmt.Sprintf(`SELECT COALESCE(NULLIF(i.category,''),'未分类') c, COUNT(*), COALESCE(SUM(i.amount),0)
		FROM invoices i WHERE %s GROUP BY c ORDER BY 3 DESC`, where), args...)
	categories := []map[string]any{}
	if err == nil {
		defer catRows.Close()
		for catRows.Next() {
			var name string
			var c int
			var a float64
			if catRows.Scan(&name, &c, &a) == nil {
				categories = append(categories, map[string]any{"name": name, "count": c, "amount": round2(a)})
			}
		}
	}

	members := []map[string]any{}
	if currentUser(r).Role == "admin" && r.URL.Query().Get("scope") != "mine" {
		memberWhere, memberArgs := s.buildFilters(r)
		memberWhere = strings.ReplaceAll(memberWhere, "i.user_id=?", "1=1")
		// 重新构造：管理端按成员汇总时忽略单个 user_id 过滤
		mq := fmt.Sprintf(`SELECT i.user_id, u.display_name, COUNT(*), COALESCE(SUM(i.amount),0),
			COALESCE(SUM(CASE WHEN i.status='pending' THEN i.amount ELSE 0 END),0)
			FROM invoices i JOIN users u ON u.id=i.user_id WHERE %s GROUP BY i.user_id ORDER BY 4 DESC`, memberWhere)
		rows, err := store.DB.Query(mq, memberArgs...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var uid int64
				var name string
				var c int
				var a, p float64
				if rows.Scan(&uid, &name, &c, &a, &p) == nil {
					members = append(members, map[string]any{
						"user_id": uid, "name": name, "count": c,
						"amount": round2(a), "pending": round2(p),
					})
				}
			}
		}
	}

	recentRows, err := store.DB.Query(invoiceSelect+"WHERE "+where+" ORDER BY i.created_at DESC, i.id DESC LIMIT 6", args...)
	recent := []model.Invoice{}
	if err == nil {
		defer recentRows.Close()
		for recentRows.Next() {
			if inv, err := scanInvoice(recentRows); err == nil {
				recent = append(recent, inv)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"totals": map[string]any{
			"count": count, "amount": round2(amount),
			"pending_amount": round2(pendingAmt), "reimbursed_amount": round2(doneAmt),
			"pending_count": pendingCnt, "reimbursed_count": doneCnt,
		},
		"months": months, "categories": categories, "members": members,
		"recent": recent, "year": year,
	})
}

// ---------------------------------------------------------------- 小工具
func clampString(s string, limit int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len([]rune(s)) > limit {
		r := []rune(s)
		s = string(r[:limit])
	}
	return s
}

func floatValue(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func toNumber(v any) any {
	switch t := v.(type) {
	case float64:
		return t
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return nil
		}
		return f
	}
	return nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func randRead(b []byte) (int, error) {
	return rand.Read(b)
}

func randomToken(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	if _, err := randRead(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

func urlEncode(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '.' || b == '-' || b == '_' {
			sb.WriteByte(b)
		} else {
			sb.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return sb.String()
}
