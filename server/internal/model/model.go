package model

import "time"

// User 团队成员 / 管理员。
type User struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	DisplayName  string     `json:"display_name"`
	PasswordHash string     `json:"-"`
	Role         string     `json:"role"`
	Dept         string     `json:"dept"`
	Active       bool       `json:"active"`
	CreatedAt    time.Time  `json:"-"`
	LastLoginAt  *time.Time `json:"-"`

	CreatedAtText   string  `json:"created_at"`
	LastLoginAtText string  `json:"last_login_at"`
	InvoiceCount    int64   `json:"invoice_count"`
	InvoiceAmount   float64 `json:"invoice_amount"`
}

// File 发票附件。
type File struct {
	ID           int64     `json:"id"`
	InvoiceID    int64     `json:"invoice_id"`
	StorageName  string    `json:"-"`
	OriginalName string    `json:"name"`
	Mime         string    `json:"mime"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"-"`
	Ext          string    `json:"ext"`
	CreatedAt    time.Time `json:"-"`
}

// Invoice 一张待报销票据。
type Invoice struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	DisplayName   string     `json:"display_name"`
	Type          string     `json:"type"`
	TypeLabel     string     `json:"type_label"`
	Status        string     `json:"status"`
	StatusLabel   string     `json:"status_label"`
	Title         string     `json:"title"`
	Category      string     `json:"category"`
	Spec          string     `json:"spec"`
	Link          string     `json:"link"`
	Qty           *float64   `json:"qty"`
	UnitPrice     *float64   `json:"unit_price"`
	Shipping      *float64   `json:"shipping"`
	Amount        float64    `json:"amount"`
	TravelTime    string     `json:"travel_time"`
	Transport     string     `json:"transport"`
	Route         string     `json:"route"`
	Traveler      string     `json:"traveler"`
	Note          string     `json:"note"`
	InvoiceNo     string     `json:"invoice_no"`
	InvoiceCode   string     `json:"invoice_code"`
	InvoiceDate   string     `json:"invoice_date"`
	Seller        string     `json:"seller"`
	Buyer         string     `json:"buyer"`
	Tax           *float64   `json:"tax"`
	Total         *float64   `json:"total"`
	InvoiceKind   string     `json:"invoice_kind"`
	OCREngine     string     `json:"ocr_engine"`
	OCRConfidence float64    `json:"ocr_confidence"`
	FieldsJSON    string     `json:"-"`
	SourceText    string     `json:"-"`
	CreatedAt     time.Time  `json:"-"`
	UpdatedAt     time.Time  `json:"-"`
	ReimbursedAt  *time.Time `json:"-"`

	CreatedAtText  string   `json:"created_at"`
	UpdatedAtText  string   `json:"updated_at"`
	ReimbursedText string   `json:"reimbursed_at"`
	Files          []File   `json:"files,omitempty"`
	Attachments    []string `json:"attachments,omitempty"`
}

// Settings 系统设置键值对。
type Settings map[string]string

// Field 识别出的单个字段，带置信度。
type Field struct {
	Value      any     `json:"value"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

// Item 票面明细行。
type Item struct {
	Name   string   `json:"name"`
	Spec   string   `json:"spec"`
	Unit   string   `json:"unit"`
	Qty    *float64 `json:"qty"`
	Price  *float64 `json:"price"`
	Amount *float64 `json:"amount"`
	Rate   *float64 `json:"rate"`
	Tax    *float64 `json:"tax"`
}

// ParseResult 离线识别的完整结果。
type ParseResult struct {
	Kind         string           `json:"kind"`
	KindLabel    string           `json:"kind_label"`
	SuggestType  string           `json:"suggest_type"`
	Fields       map[string]Field `json:"fields"`
	Items        []Item           `json:"items"`
	Travel       map[string]any   `json:"travel"`
	Suggest      map[string]any   `json:"suggest"`
	Warnings     []string         `json:"warnings"`
	RawText      string           `json:"raw_text"`
	Engine       string           `json:"engine"`
	Pages        int              `json:"pages"`
	ElapsedMS    int64            `json:"elapsed_ms"`
	Confidence   float64          `json:"confidence"`
	FileName     string           `json:"filename"`
	TokenCount   int              `json:"token_count"`
}
