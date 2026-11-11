package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"invoice/server/internal/model"
)

//go:embed schema.sql
var schemaSQL string

// DB 全局连接池。
var DB *sql.DB

// 默认设置项。
var defaults = map[string]string{
	"org_name":          "我的团队",
	"currency":          "CNY",
	"retention_mode":    "permanent",
	"retention_days":    "365",
	"retention_scope":   "files",
	"max_upload_mb":     "20",
	"allowed_exts":      "pdf,jpg,jpeg,png,webp,bmp",
	"categories":        "办公用品,电子设备,软件服务,差旅交通,餐饮招待,通讯网络,物流运输,其他",
	"ocr_engine":        "auto",
	"default_type":      "procurement",
	"reimbursed_notice": "",
}

// Open 连接 MySQL 并完成建表、初始化设置。
func Open(dsn string) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开数据库失败: %w", err)
	}
	db.SetMaxOpenConns(24)
	db.SetMaxIdleConns(12)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)

	deadline := time.Now().Add(15 * time.Second)
	for {
		if err = db.Ping(); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("连接 MySQL 失败: %w", err)
		}
		time.Sleep(500 * time.Millisecond)
	}
	DB = db

	for _, stmt := range splitStatements(schemaSQL) {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("建表失败: %w\nSQL: %s", err, stmt)
		}
	}
	// 老库补列：CREATE TABLE IF NOT EXISTS 不会给已存在的表加字段。
	if err := ensureColumn("invoices", "unit", "VARCHAR(16) NOT NULL DEFAULT '' AFTER spec"); err != nil {
		return fmt.Errorf("升级表结构失败: %w", err)
	}
	if err := seedSettings(); err != nil {
		return err
	}
	log.Printf("数据库就绪")
	return nil
}

// ensureColumn 幂等地给已有表补列。
func ensureColumn(table, column, ddl string) error {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&n)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err = DB.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, column, ddl))
	return err
}

func splitStatements(sqlText string) []string {
	// 去掉整行的 "--" 注释，再按分号切分，避免把带注释的第一段整条丢掉。
	var cleaned strings.Builder
	for _, line := range strings.Split(sqlText, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		cleaned.WriteString(line)
		cleaned.WriteString("\n")
	}
	var out []string
	for _, part := range strings.Split(cleaned.String(), ";") {
		s := strings.TrimSpace(part)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func seedSettings() error {
	now := TimeText(time.Now())
	for k, v := range defaults {
		_, err := DB.Exec("INSERT IGNORE INTO settings(`key`, `value`, updated_at) VALUES(?,?,?)", k, v, now)
		if err != nil {
			return fmt.Errorf("初始化设置失败: %w", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- 通用助手
// TimeText 统一时间格式，前端直接展示。
func TimeText(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// NullTime 安全解引用。
func NullTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return TimeText(*t)
}

// GetSettings 读取全部设置（合并默认值）。
func GetSettings() model.Settings {
	out := model.Settings{}
	for k, v := range defaults {
		out[k] = v
	}
	if DB == nil {
		return out
	}
	rows, err := DB.Query("SELECT `key`, `value` FROM settings")
	if err != nil {
		log.Printf("读取设置失败: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			out[k] = v
		}
	}
	return out
}

// GetSetting 读取单个设置（数据库未就绪时回退默认值，避免空指针）。
func GetSetting(key string) string {
	if DB == nil {
		return defaults[key]
	}
	var v string
	if err := DB.QueryRow("SELECT `value` FROM settings WHERE `key`=?", key).Scan(&v); err != nil {
		return defaults[key]
	}
	return v
}

// SetSettings 批量写入设置。
func SetSettings(values map[string]string) error {
	now := TimeText(time.Now())
	for k, v := range values {
		if _, err := DB.Exec("INSERT INTO settings(`key`,`value`,updated_at) VALUES(?,?,?) "+
			"ON DUPLICATE KEY UPDATE `value`=VALUES(`value`), updated_at=VALUES(updated_at)", k, v, now); err != nil {
			return err
		}
	}
	return nil
}

// Audit 写操作日志。
func Audit(userID *int64, actor, action, target string, detail any, ip string) {
	detailText := ""
	switch v := detail.(type) {
	case nil:
	case string:
		detailText = v
	default:
		if b, err := json.Marshal(v); err == nil {
			detailText = string(b)
		} else {
			detailText = fmt.Sprintf("%v", v)
		}
	}
	if len(detailText) > 2000 {
		detailText = detailText[:2000]
	}
	_, err := DB.Exec("INSERT INTO audit_logs(user_id, actor_name, action, target, detail, ip, created_at) "+
		"VALUES(?,?,?,?,?,?,?)", userID, actor, action, target, detailText, ip, TimeText(time.Now()))
	if err != nil {
		log.Printf("写日志失败: %v", err)
	}
}
