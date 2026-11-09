#!/usr/bin/env bash
# 初始化 MySQL 数据库与应用账号（Ubuntu / Debian，使用 sudo mysql 走 auth_socket）
set -euo pipefail

DB_NAME="${INVOICE_DB:-invoice}"
DB_USER="${INVOICE_DB_USER:-invoice}"
DB_PASS="${INVOICE_DB_PASS:-Invoice@2026}"

echo "==> 创建数据库 $DB_NAME"
sudo mysql <<SQL
CREATE DATABASE IF NOT EXISTS \`$DB_NAME\`
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
CREATE USER IF NOT EXISTS '$DB_USER'@'localhost' IDENTIFIED WITH mysql_native_password BY '$DB_PASS';
CREATE USER IF NOT EXISTS '$DB_USER'@'%' IDENTIFIED WITH mysql_native_password BY '$DB_PASS';
GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'localhost';
GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'%';
FLUSH PRIVILEGES;
SQL

echo "    -> 账号：$DB_USER / $DB_PASS"
echo "    -> DSN：$DB_USER:$DB_PASS@tcp(127.0.0.1:3306)/$DB_NAME?charset=utf8mb4&parseTime=true&loc=Local"
