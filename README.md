# running_page_go

用 Go 语言重写 [running_page](https://github.com/Jason-cqtan/running_page) 的所有 Python 数据同步脚本。前端（TypeScript/React/Vite）保持不变，Go 只负责数据同步和处理后端部分。

---

## 项目结构

```
running_page_go/
├── cmd/
│   ├── garmin_sync/main.go   # Garmin 同步 CLI 入口
│   ├── strava_sync/main.go   # Strava 同步 CLI 入口
│   └── keep_sync/main.go     # Keep 同步 CLI 入口
├── config/config.go          # 路径和常量配置
├── db/db.go                  # SQLite 封装（无 CGO）
├── sync/
│   ├── garmin.go             # Garmin Connect API 客户端
│   ├── strava.go             # Strava OAuth2 + 活动拉取
│   └── keep.go               # Keep 登录 + AES 解密 + 坐标转换
├── generator/
│   ├── generator.go          # SQLite 读写 + activities.json 生成
│   ├── gpx.go                # GPX 文件解析
│   └── tcx.go                # TCX 文件解析
├── utils/
│   ├── utils.go              # 时区处理等工具函数
│   └── polyline.go           # Google Polyline 编解码
├── activities/               # activities.json 输出目录
├── GPX_OUT/                  # GPX 文件存储目录
├── TCX_OUT/                  # TCX 文件存储目录
├── FIT_OUT/                  # FIT 文件存储目录
├── go.mod
├── Makefile
└── README.md
```

---

## 依赖安装

需要 Go 1.22+。

```bash
go mod tidy
```

---

## 构建

```bash
make build
# 或单独构建某个工具
go build -o bin/garmin_sync ./cmd/garmin_sync
go build -o bin/strava_sync ./cmd/strava_sync
go build -o bin/keep_sync ./cmd/keep_sync
```

---

## 使用说明

### Garmin

从 Garmin Connect 同步活动数据（需要 garth 格式的 secret_string）：

```bash
# 同步所有活动（GPX 格式）
./bin/garmin_sync --secret '{"oauth2_token": {"access_token": "..."}}' 

# 仅同步跑步，使用中国服务器，下载 TCX 格式
./bin/garmin_sync --secret '...' --cn --only-run --type tcx

# 使用 Makefile
make garmin ARGS="--secret '...' --only-run"
```

参数说明：
| 参数 | 说明 | 默认值 |
|---|---|---|
| `--secret` | garth secret string（JSON） | 必填 |
| `--cn` | 使用 Garmin 中国服务器 | false |
| `--only-run` | 只同步跑步活动 | false |
| `--type` | 文件格式：gpx/tcx/fit | gpx |

---

### Strava

通过 OAuth2 Refresh Token 同步 Strava 活动：

```bash
./bin/strava_sync \
  --client-id YOUR_CLIENT_ID \
  --client-secret YOUR_CLIENT_SECRET \
  --refresh-token YOUR_REFRESH_TOKEN

# 只同步跑步，并指定起始日期
./bin/strava_sync --client-id ... --client-secret ... --refresh-token ... \
  --only-run --after 2024-01-01
```

参数说明：
| 参数 | 说明 | 默认值 |
|---|---|---|
| `--client-id` | Strava Client ID | 必填 |
| `--client-secret` | Strava Client Secret | 必填 |
| `--refresh-token` | Strava Refresh Token | 必填 |
| `--only-run` | 只同步跑步活动 | false |
| `--after` | 只同步该日期之后的活动（YYYY-MM-DD） | 空（全部） |

---

### Keep

从 Keep App 同步运动数据：

```bash
./bin/keep_sync --phone 13800138000 --password yourpassword

# 同步多种运动类型，不保存 GPX 文件
./bin/keep_sync --phone ... --password ... \
  --sync-types "running,hiking,cycling" --with-gpx=false
```

参数说明：
| 参数 | 说明 | 默认值 |
|---|---|---|
| `--phone` | Keep 手机号 | 必填 |
| `--password` | Keep 密码 | 必填 |
| `--sync-types` | 运动类型，逗号分隔 | running |
| `--with-gpx` | 是否保存 GPX 文件 | true |

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

---

## 数据格式

同步完成后会生成：
- `data.db`：SQLite 数据库，包含所有活动记录
- `activities/activities.json`：JSON 格式的活动列表（与原 Python 版本格式兼容，前端可直接使用）