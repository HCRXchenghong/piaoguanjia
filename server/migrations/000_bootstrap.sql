-- 初始化数据库与账号（MySQL 8.0）
ALTER USER 'root'@'localhost' IDENTIFIED WITH mysql_native_password BY 'Root@Invoice2026';

CREATE DATABASE IF NOT EXISTS invoice
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

CREATE USER IF NOT EXISTS 'invoice'@'localhost' IDENTIFIED WITH mysql_native_password BY 'Invoice@2026';
CREATE USER IF NOT EXISTS 'invoice'@'%' IDENTIFIED WITH mysql_native_password BY 'Invoice@2026';

GRANT ALL PRIVILEGES ON invoice.* TO 'invoice'@'localhost';
GRANT ALL PRIVILEGES ON invoice.* TO 'invoice'@'%';
FLUSH PRIVILEGES;
