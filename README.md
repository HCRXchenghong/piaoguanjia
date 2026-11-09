# 票管家 · 团队发票报销管理平台

![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)
![Vue](https://img.shields.io/badge/Vue-3.5-42B883?logo=vue.js&logoColor=white)
![MySQL](https://img.shields.io/badge/MySQL-8.0-4479A1?logo=mysql&logoColor=white)
![OCR](https://img.shields.io/badge/OCR-离线识别-2F855A)

面向小团队的发票收集与报销台账系统：成员一人一个账号，上传图片 / PDF 发票后由**本地离线 OCR**自动识别票面信息并填单，确认后进入台账；管理员可以查看全员数据、维护报销状态，并一键导出「附件重命名 + Excel 汇总」的压缩包。

> 技术栈：**Go 1.23+ / MySQL 8.0 / Vue 3 / Arco Design Vue / ECharts**
> 识别引擎：**Windows 内置 OCR（Windows.Media.Ocr）+ 自研 PDF 文字层解析**，全程不联网、不上传第三方。

![概览](docs/screenshots/02-概览.png)

## 目录

- [1. 快速开始](#1-快速开始)
- [2. 目录结构](#2-目录结构)
- [3. 功能清单](#3-功能清单)
- [4. 配置](#4-配置)
- [5. 常见问题](#5-常见问题)
- [6. 开发模式](#6-开发模式)
- [7. 界面预览](#7-界面预览)
- [8. 文档](#8-文档)

---

## 1. 快速开始

仓库只保留源码，不提交编译产物和构建结果，所以**第一次运行要先完成构建**。

前置条件：

| 组件 | 版本 | 用途 |
|---|---|---|
| MySQL | 8.0+ | 数据存储，默认监听 `127.0.0.1:3306` |
| Go | 1.23+ | 仅编译后端时需要 |
| Node.js + npm | 18+ | 仅构建前端时需要 |
| OCR 引擎 | 见[常见问题](#5-常见问题) | 图片 / 扫描件识别，PDF 文字层不需要 |

### Windows

1. **编译后端**：双击 `构建后端.bat`，或执行
   ```powershell
   cd server
   go build -o bin\invoice-server.exe ./cmd/server
   ```
2. **构建前端**：双击 `构建前端.bat`，产物输出到 `web\dist`。
3. **启动**：双击 `启动.bat`，脚本会检查 3306 端口、按需拉起 MySQL、启动服务并打开浏览器。

`启动.bat` 默认按本机 MySQL 安装目录查找 `mysqld.exe`；路径不同时，修改脚本里的 `INVOICE_MYSQL_HOME`，或提前手动启动 MySQL。

首次访问会进入「初始化」页，创建第一个管理员账号（团队名称、用户名、密码），之后即可在「成员管理」里给同事开账号。局域网内其他电脑用 `http://<本机IP>:8686` 访问。

停止服务：双击 `停止服务.bat`，或在服务窗口按 `Ctrl+C`。

### Ubuntu / Debian

```bash
# 1) 依赖：MySQL + 离线 OCR（Tesseract 中文包）
sudo apt update
sudo apt install -y mysql-server tesseract-ocr tesseract-ocr-chi-sim

# 2) 建库与账号：通过 INVOICE_DB_PASS 指定密码（请勿沿用脚本里的示例口令）
INVOICE_DB_PASS='<设置一个强密码>' bash deploy/init-db.sh

# 3) 编译后端 + 构建前端（需要 Go 1.23+ 与 Node 18+）
chmod +x start.sh deploy/*.sh
bash deploy/build-linux.sh

# 4) 启动
./start.sh

# 5) 开机自启（可选）
sudo cp deploy/invoice-hub.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now invoice-hub
```

如果建库时用了自定义密码，启动时用 `INVOICE_DSN` 覆盖连接串：

```bash
INVOICE_DSN="invoice:<强密码>@tcp(127.0.0.1:3306)/invoice?charset=utf8mb4&parseTime=true&loc=Local" ./start.sh
```

---

## 2. 目录结构

```
piaoguanjia/
├─ 启动.bat                # 一键启动（MySQL + 服务 + 打开浏览器）
├─ 停止服务.bat
├─ 构建前端.bat            # npm install + npm run build
├─ 构建后端.bat            # go build -> server/bin/invoice-server.exe
├─ start.sh                # Linux / macOS 启动脚本
├─ deploy/                 # 建库脚本、Linux 构建、systemd 服务单元
├─ server/                 # Go 后端
│  ├─ cmd/server/main.go   # 入口：配置、启动、优雅退出
│  ├─ internal/
│  │  ├─ api/              # 路由、鉴权中间件、HTTP handler
│  │  ├─ auth/             # PBKDF2 口令 + 自签 JWT
│  │  ├─ config/           # 配置与数据目录
│  │  ├─ exporter/         # ZIP 打包 + Excel（excelize）
│  │  ├─ model/            # 数据模型
│  │  ├─ ocr/              # 离线识别：PDF 解析 / WinOCR / 票面抽取
│  │  ├─ storage/          # 附件落盘、缩略图、保留期清理
│  │  └─ store/            # MySQL 连接、建表、设置、审计日志
│  ├─ migrations/          # 初始化库与账号 SQL
│  ├─ bin/                 # 编译产物（不提交）
│  └─ data/                # 运行时数据：附件、临时文件、缩略图、日志（不提交）
├─ web/                    # Vue 3 + Vite + Arco Design Vue
│  ├─ src/views/           # 登录、概览、上传向导、票据、成员、导出、设置
│  ├─ src/components/      # 票据详情抽屉等
│  └─ dist/                # 构建产物（不提交，由 Go 直接托管）
├─ docs/                   # 方案说明、接口文档、截图
├─ samples/                # 示例发票，可直接拖进上传页试用
└─ tools/                  # 运维小工具
```

---

## 3. 功能清单

**成员端**

- 上传图片（JPG/PNG/WEBP/BMP）或 PDF，自动识别：发票号码、开票日期、销售方、购买方、价税合计、税额、明细行（名称/规格/数量/单价/金额/税率）
- 三步向导：上传识别 → 补充信息（带「已识别」标记）→ 核对确认 → 提交
- 采购报销 / 出行报销两套表单，可随时切换
- 「我的发票」按全部 / 未报销 / 已报销筛选，支持按月份、类型、关键词检索
- 自己更新报销状态、查看附件、删除票据

**管理端**

- 数据概览：年度总额、待报销、已报销、团队规模，月度柱状图 + 类别环形图 + 成员排行
- 全部票据：按成员、状态、类型、月份、关键词筛选，行内可直接改报销状态
- 成员管理：新建 / 编辑 / 重置密码 / 停用 / 删除
- 导出中心：按月份 / 年份 / 自定义区间 / 全部导出，可叠加类型、状态、成员筛选
- 系统设置：团队名称、消费类别、上传限制、识别开关、保留策略、备份、审计日志

**导出包结构**

```
发票汇总_2026年10月_未报销_20261109_1602.zip
├─ 附件001_办公用品_242.95元.pdf      ← 原件按「附件序号_类别_金额元」重命名
├─ 附件002_差旅交通_74.50元.png
├─ 01_采购报销明细.xlsx                ← 明细 / 汇总 / 类别统计
├─ 02_出行报销明细.xlsx                ← 明细 / 汇总 / 交通工具统计（没有出行数据时不生成）
└─ 导出说明.txt
```

采购与出行**分成两个 Excel**，列各自贴合业务；哪一类没有数据就不生成对应文件。

Excel 的「附件文件」列与压缩包里的文件名一一对应；「汇总」按上报人统计待报销 / 已报销金额，「分类统计」按类别给出金额与占比。

---

## 4. 配置

后端启动参数（也可用环境变量）：

| 参数 | 默认值 | 环境变量 | 说明 |
|---|---|---|---|
| `-addr` | `:8686` | `INVOICE_ADDR` | 监听地址，`:8686` 表示所有网卡 |
| `-dsn` | `invoice:<密码>@tcp(127.0.0.1:3306)/invoice?...` | `INVOICE_DSN` | MySQL 连接串 |
| `-data` | `data` | `INVOICE_DATA` | 附件与临时文件目录 |
| `-web` | `../web/dist` | `INVOICE_WEB` | 前端静态资源目录 |
| `-debug` | 关 | `INVOICE_DEBUG=1` | 打印所有请求日志 |

数据库账号（见 `server/migrations/000_bootstrap.sql` 与 `deploy/init-db.sh`）：

- 应用账号 `invoice`，库 `invoice`；密码在建库时通过 `INVOICE_DB_PASS` 指定，脚本内置的示例口令请务必替换；
- `root` 账号仅限本机运维使用，请单独设置强口令。

> 生产环境请修改这两处密码，并同步修改 `-dsn`（或 `INVOICE_DSN`）与 `启动.bat` 里的 MySQL 路径。

---

## 5. 常见问题

**Q：启动后页面打不开 / 显示「前端尚未构建」？**

先执行一次 `构建前端.bat`（或 `cd web && npm run build`），确保 `web/dist/index.html` 存在；后端需要先编译出 `server/bin/invoice-server.exe`。

**Q：提示连不上 MySQL？**

看 `启动.bat` 是否成功拉起 MySQL；手动检查：

```bash
mysql -u invoice -p -h 127.0.0.1 invoice -e "select 1"
```

若数据目录损坏，可删除 MySQL 数据目录后重新初始化（会清空数据）。

**Q：识别没反应 / 提示 OCR 不可用？**

| 平台 | 引擎 | 安装方式 |
|---|---|---|
| Windows | Windows 内置 OCR | 系统自带；若无中文包：设置 → 时间和语言 → 语言 → 中文 → 选项 → 安装「光学字符识别」 |
| Ubuntu/Debian | Tesseract | `sudo apt install tesseract-ocr tesseract-ocr-chi-sim` |
| macOS | Tesseract | `brew install tesseract tesseract-lang` |

识别引擎在「系统设置 → 上传与识别」里可见并可切换（自动 / Windows 内置 / Tesseract / 关闭）。
**PDF 文字层不依赖任何 OCR 引擎**，电子发票即使没有 OCR 也能准确解析。

**Q：端口 8686 被占用？**

```powershell
server\bin\invoice-server.exe -addr :8687 -web web\dist
```

**Q：怎么备份 / 迁移？**

「系统设置 → 下载完整备份」会打包 `data.json`（用户、票据、设置）与全部附件；迁移时在新机器恢复数据库与 `server/data/files` 即可。

**Q：数据保留多久？**

默认永久保存。可在「系统设置 → 存储与保留」改为按天自动清理，并选择「只删原件保留台账」或「连记录一起删」。

---

## 6. 开发模式

```powershell
# 后端（热重启随意）
cd server
go run ./cmd/server -addr :8686 -web ../web/dist

# 前端（Vite 开发服务器，已配置 /api 代理到 8686）
cd web
npm run dev        # http://127.0.0.1:5173
```

---

## 7. 界面预览

| 首次初始化 | 登录 |
|---|---|
| ![首次初始化](docs/screenshots/00-首次初始化.png) | ![登录](docs/screenshots/01-登录.png) |
| **数据概览** | **上传识别** |
| ![概览](docs/screenshots/02-概览.png) | ![上传识别](docs/screenshots/03-上传识别.png) |
| **补充信息** | **确认提交** |
| ![补充信息](docs/screenshots/04-补充信息.png) | ![确认提交](docs/screenshots/05-确认提交.png) |
| **票据详情** | **票据列表** |
| ![票据详情](docs/screenshots/06-票据详情.png) | ![票据列表](docs/screenshots/07-票据列表.png) |
| **成员管理** | **导出中心** |
| ![成员管理](docs/screenshots/08-成员管理.png) | ![导出中心](docs/screenshots/09-导出中心.png) |
| **系统设置** | **成员工作台** |
| ![系统设置](docs/screenshots/10-系统设置.png) | ![成员工作台](docs/screenshots/11-成员工作台.png) |

> 截图与 `samples/` 中的示例发票均为演示数据，不含真实票据信息。

---

## 8. 文档

| 文档 | 内容 |
|---|---|
| [方案说明](docs/方案说明.md) | 设计目标、总体架构、技术选型、数据模型、离线识别链路、权限与导出设计 |
| [接口文档](docs/接口文档.md) | HTTP API 一览、请求 / 响应示例、查询参数与错误码 |
| [文档索引](docs/README.md) | 文档与截图目录说明 |
