-- 票管家 · MySQL 8.0 表结构
CREATE TABLE IF NOT EXISTS users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username      VARCHAR(64)  NOT NULL,
  display_name  VARCHAR(64)  NOT NULL DEFAULT '',
  password_hash VARCHAR(255) NOT NULL,
  role          VARCHAR(16)  NOT NULL DEFAULT 'member',
  active        TINYINT(1)   NOT NULL DEFAULT 1,
  dept          VARCHAR(64)  NOT NULL DEFAULT '',
  created_at    DATETIME     NOT NULL,
  last_login_at DATETIME     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS invoices (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id        BIGINT UNSIGNED NOT NULL,
  type           VARCHAR(16)  NOT NULL DEFAULT 'procurement',
  status         VARCHAR(16)  NOT NULL DEFAULT 'pending',
  title          VARCHAR(200) NOT NULL DEFAULT '',
  category       VARCHAR(64)  NOT NULL DEFAULT '',
  spec           VARCHAR(200) NOT NULL DEFAULT '',
  link           VARCHAR(500) NOT NULL DEFAULT '',
  qty            DECIMAL(12,2) NULL,
  unit_price     DECIMAL(12,2) NULL,
  shipping       DECIMAL(12,2) NOT NULL DEFAULT 0,
  amount         DECIMAL(12,2) NOT NULL DEFAULT 0,
  travel_time    VARCHAR(40)  NOT NULL DEFAULT '',
  transport      VARCHAR(32)  NOT NULL DEFAULT '',
  route          VARCHAR(200) NOT NULL DEFAULT '',
  traveler       VARCHAR(64)  NOT NULL DEFAULT '',
  note           VARCHAR(2000) NOT NULL DEFAULT '',
  invoice_no     VARCHAR(64)  NOT NULL DEFAULT '',
  invoice_code   VARCHAR(64)  NOT NULL DEFAULT '',
  invoice_date   VARCHAR(20)  NOT NULL DEFAULT '',
  seller         VARCHAR(200) NOT NULL DEFAULT '',
  buyer          VARCHAR(200) NOT NULL DEFAULT '',
  tax            DECIMAL(12,2) NULL,
  total          DECIMAL(12,2) NULL,
  invoice_kind   VARCHAR(64)  NOT NULL DEFAULT '',
  ocr_engine     VARCHAR(32)  NOT NULL DEFAULT '',
  ocr_confidence DECIMAL(4,2) NOT NULL DEFAULT 0,
  fields_json    LONGTEXT     NULL,
  source_text    LONGTEXT     NULL,
  created_at     DATETIME     NOT NULL,
  updated_at     DATETIME     NOT NULL,
  reimbursed_at  DATETIME     NULL,
  reimbursed_by  BIGINT UNSIGNED NULL,
  deleted        TINYINT(1)   NOT NULL DEFAULT 0,
  deleted_at     DATETIME     NULL,
  PRIMARY KEY (id),
  KEY idx_inv_user (user_id, deleted),
  KEY idx_inv_status (status, deleted),
  KEY idx_inv_date (invoice_date),
  KEY idx_inv_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS files (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  invoice_id    BIGINT UNSIGNED NOT NULL,
  storage_name  VARCHAR(128) NOT NULL,
  original_name VARCHAR(255) NOT NULL DEFAULT '',
  mime          VARCHAR(64)  NOT NULL DEFAULT '',
  size          BIGINT       NOT NULL DEFAULT 0,
  sha256        VARCHAR(64)  NOT NULL DEFAULT '',
  kind          VARCHAR(16)  NOT NULL DEFAULT 'original',
  created_at    DATETIME     NOT NULL,
  deleted       TINYINT(1)   NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_files_invoice (invoice_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS pending_uploads (
  token      VARCHAR(64)  NOT NULL,
  user_id    BIGINT UNSIGNED NOT NULL,
  filename   VARCHAR(255) NOT NULL DEFAULT '',
  path       VARCHAR(500) NOT NULL DEFAULT '',
  mime       VARCHAR(64)  NOT NULL DEFAULT '',
  size       BIGINT       NOT NULL DEFAULT 0,
  sha256     VARCHAR(64)  NOT NULL DEFAULT '',
  parse_json LONGTEXT     NULL,
  created_at DATETIME     NOT NULL,
  PRIMARY KEY (token),
  KEY idx_pending_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS audit_logs (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  user_id    BIGINT UNSIGNED NULL,
  actor_name VARCHAR(64)  NOT NULL DEFAULT '',
  action     VARCHAR(64)  NOT NULL,
  target     VARCHAR(128) NOT NULL DEFAULT '',
  detail     TEXT         NULL,
  ip         VARCHAR(64)  NOT NULL DEFAULT '',
  created_at DATETIME     NOT NULL,
  PRIMARY KEY (id),
  KEY idx_audit_time (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS settings (
  `key`      VARCHAR(64)  NOT NULL,
  `value`    TEXT         NOT NULL,
  updated_at DATETIME     NOT NULL,
  PRIMARY KEY (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
