# running_page_go

Go 版 Running Page —— 用 Go 语言替代 Python 作为后端数据同步脚本，前端保持 **Vite + React + TypeScript + Tailwind CSS** 技术栈不变。

## 🌐 在线预览

👉 [https://jason-cqtan.github.io/running_page_go/](https://jason-cqtan.github.io/running_page_go/)

---

## 项目结构

```
running_page_go/
├── .github/
│   └── workflows/
│       ├── sync_garmin.yml       # Garmin 数据同步 workflow
│       ├── sync_strava.yml       # Strava 数据同步 workflow
│       ├── sync_keep.yml         # Keep 数据同步 workflow
│       └── deploy_pages.yml      # 构建前端并部署到 GitHub Pages
│
├── cmd/                          # Go CLI 工具
│   ├── garmin_sync/main.go       # Garmin 同步 CLI 入口
│   ├── strava_sync/main.go       # Strava 同步 CLI 入口
│   └── keep_sync/main.go         # Keep 同步 CLI 入口
├── config/config.go              # 路径和常量配置
├── db/db.go                      # SQLite 封装（无 CGO）
├── sync/                         # Go 同步逻辑
├── generator/                    # Go 数据生成器
├── utils/                        # Go 工具函数
│
├── src/                          # 前端 React/TypeScript 源码
│   ├── main.tsx
│   ├── pages/
│   ├── components/
│   ├── hooks/
│   ├── utils/
│   ├── styles/
│   └── static/
│       └── activities.json       # 由 Go 同步脚本生成，前端读取
│
├── public/                       # 前端静态文件
├── assets/                       # 图片及 SVG 资源
│
├── index.html                    # Vite 入口
├── vite.config.ts
├── tsconfig.json
├── package.json
├── tailwind.config.js
├── postcss.config.js
├── go.mod
└── Makefile
```

---

## 🚀 快速开始

### 前端开发

```bash
# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 构建生产版本
npm run build
```

### Go 工具

需要 Go 1.22+。

```bash
# 安装依赖
go mod tidy

# 构建所有工具
make build

# 或单独构建
go build -o bin/garmin_sync ./cmd/garmin_sync
go build -o bin/strava_sync ./cmd/strava_sync
go build -o bin/keep_sync ./cmd/keep_sync
```

---

## 🔧 各平台数据同步

### Garmin

从 Garmin Connect 同步活动数据：

```bash
./bin/garmin_sync \
  --secret '{"oauth2_token": {"access_token": "..."}}' \
  --cn=false \
  --type=gpx
```

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--secret` | garth secret string（JSON） | 必填 |
| `--cn` | 使用 Garmin 中国服务器 | false |
| `--only-run` | 只同步跑步活动 | false |
| `--type` | 文件格式：gpx/tcx/fit | gpx |

### Strava

通过 OAuth2 Refresh Token 同步 Strava 活动：

```bash
./bin/strava_sync \
  --client-id YOUR_CLIENT_ID \
  --client-secret YOUR_CLIENT_SECRET \
  --refresh-token YOUR_REFRESH_TOKEN
```

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--client-id` | Strava Client ID | 必填 |
| `--client-secret` | Strava Client Secret | 必填 |
| `--refresh-token` | Strava Refresh Token | 必填 |
| `--only-run` | 只同步跑步活动 | false |
| `--after` | 只同步该日期之后的活动（YYYY-MM-DD） | 空（全部） |

### Keep

从 Keep App 同步运动数据：

```bash
./bin/keep_sync \
  --phone 13800138000 \
  --password yourpassword \
  --sync-types "running"
```

| 参数 | 说明 | 默认值 |
|---|---|---|
| `--phone` | Keep 手机号 | 必填 |
| `--password` | Keep 密码 | 必填 |
| `--sync-types` | 运动类型，逗号分隔 | running |
| `--with-gpx` | 是否保存 GPX 文件 | true |

---

## ⚙️ GitHub Actions 配置

### 启用 GitHub Pages

1. 进入仓库的 **Settings** → **Pages**
2. 在 **Source** 下选择 **GitHub Actions**
3. 保存后，每次推送到 `main` 分支将自动触发部署

### 需要配置的 Secrets

在仓库的 **Settings** → **Secrets and variables** → **Actions** 中添加以下 Secrets：

#### Garmin（`sync_garmin.yml`）
| Secret 名称 | 说明 |
|---|---|
| `GARMIN_SECRET_STRING` | garth 格式的 Garmin secret JSON 字符串 |
| `GARMIN_IS_CN` | 是否使用中国服务器（`true` 或 `false`） |

#### Strava（`sync_strava.yml`）
| Secret 名称 | 说明 |
|---|---|
| `STRAVA_CLIENT_ID` | Strava 应用 Client ID |
| `STRAVA_CLIENT_SECRET` | Strava 应用 Client Secret |
| `STRAVA_REFRESH_TOKEN` | Strava OAuth2 Refresh Token |

#### Keep（`sync_keep.yml`）
| Secret 名称 | 说明 |
|---|---|
| `KEEP_PHONE` | Keep 账号手机号 |
| `KEEP_PASSWORD` | Keep 账号密码 |

### Workflow 触发逻辑

- **`deploy_pages.yml`**：当 `main` 分支的前端文件更新，或任一同步 workflow 完成时自动触发，也支持手动触发
- **`sync_garmin.yml`**：每天 UTC 00:00 自动运行，支持手动触发
- **`sync_strava.yml`**：每天 UTC 00:30 自动运行，支持手动触发
- **`sync_keep.yml`**：每天 UTC 01:00 自动运行，支持手动触发

---

## 与 Python 版本对比

| 特性 | Python 版 | Go 版 |
|---|---|---|
| 运行方式 | 需要 Python 环境 + pip 安装依赖 | 单一静态二进制，无需运行时 |
| 性能 | 单线程/asyncio | goroutine 并发，下载速度更快 |
| 内存占用 | 较高 | 较低 |
| 部署 | 需要 Python 3.x + 虚拟环境 | 直接运行二进制，跨平台 |
| SQLite 依赖 | 需要 C 扩展 | 纯 Go 实现（modernc.org/sqlite），无 CGO |
| 类型安全 | 动态类型 | 静态类型，编译期错误检查 |
| 并发下载 | asyncio（最多 10 个） | goroutine + semaphore（可配置） |
