# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project

**优舞盟（dance-elite）** — 少儿中国舞赛事系统：微信小程序端（报名/支付/资料上传/榜单）、PC 后台管理端、免登录音频审核 H5。

配套设计文档（在 `docs/` 下，改动涉及业务规则时必须先读）：
- `docs/API接口设计.md` — 接口规范、错误码、三类端（小程序 `/api/v1`、后台 `/api/v1/admin`、H5 `/h5`）
- `docs/数据库表结构设计.md` — ER 关系与设计决策（先读第 0 节）
- `docs/优舞盟报名功能.md` — 报名流程与状态机、已确认的关键业务决策
- `docs/小程序页面/优舞盟小程序页面与功能设计.md`、`docs/后台管理/优舞盟后台管理页面与功能设计.md`

## Tech Stack

| Layer | Technology | Version |
|-------|------------|---------|
| Language | Go | 1.26.x |
| HTTP | Gin | v1.12 |
| ORM | GORM | v2（driver: sqlite） |
| Database | SQLite | 单机部署，数据量小 |
| Frontend（后台/H5） | React + TypeScript + Vite | 19.x / 5.x |
| Config | godotenv + 环境变量 | — |
| Auth | golang-jwt/jwt v5 | — |
| Excel | excelize/v2 | — |
| Image | disintegration/imaging | — |
| Log | go.uber.org/zap | — |

Do not upgrade major versions or add new major dependencies without explicit approval.

## Commands

```bash
# Run (dev)
go run ./cmd/dance-elite

# Build & test
go build ./...
go test ./...
go vet ./...
gofmt -w .

# Docker 部署（Aliyun）
bash scripts/build.sh && bash scripts/push-aliyun.sh
```

配置：无 YAML、无命令行参数。全部配置来自环境变量：本地开发由 godotenv 读取 `.env`（可选，已存在的环境变量优先），生产由容器运行时注入；必需项缺失直接 fail-fast。复制 `.env.example` 为 `.env` 用于本地开发（`.env` 不入库）。

## Directory Structure

```
/
├── .env.example            # 环境变量模板（.env 不入库）
├── cmd/
│   ├── dance-elite/        # 服务入口：main.go 配置与日志，run.go 依赖装配（composition root）
├── internal/
│   ├── router/             # Gin 路由注册 + handler（可 httptest 直接测）
│   ├── service/            # 业务逻辑层（依赖注入 store，可注入 fake 测试）
│   ├── store/              # 数据访问层（SQLite：db.go 连接池 / migrate.go 迁移 / tx.go 事务）
│   ├── model/              # 数据模型与 DTO（request / response）
│   ├── configs/            # 配置加载（默认值 + .env + 环境变量，启动自检）
│   ├── middleware/         # Gin 中间件（RequestID / AccessLog / Recovery）
│   ├── pkg/httpx/          # 统一响应包装 + 全局错误码 + HandleErrors
│   ├── pkg/logger/         # zap 日志封装
│   └── server/             # http.Server 封装（context 驱动优雅关闭，与框架解耦）
├── dance-admin/            # 后台管理端 + 音频审核 H5 前端（Ant Design Pro / React，独立前端工程）
├── scripts/                # Dockerfile、build/push 脚本
└── data/                   # SQLite 数据文件（不入库）
```

依赖方向：`cmd → router → service → store → model`。`internal/` 包之间只允许上层依赖下层，禁止反向。

## API Conventions（以 docs/API接口设计.md 为准）

- 基础路径：小程序端 `/api/v1`；后台 `/api/v1/admin`；音频审核 H5 `/h5`
- 字段命名 **snake_case**；时间 ISO 8601 本地时间（`2027-01-01T10:00:00+08:00`）；金额单位**分**（整数）
- 枚举值一律英文小写蛇形（`classical`/`folk`），中文仅管理端展示层转换
- 统一响应包装：`{"code": 0, "message": "success", "data": {}}`
- 错误码：0 成功；40001 参数错；40101 未登录；40102 账号不存在（后台登录）；40103 密码错误（后台登录）；40104 账号已锁定（后台登录）；40301 无权限；40302 账号已停用（后台登录）；40401 不存在；40901 业务冲突；42901 限流；50001 服务器错误；报名域 41001–41007（证件号格式、年龄、重复报名、名额已满、非报名期等）、41012（赛事状态不允许该操作）
- 分页：query `page`（默认 1）/ `page_size`（默认 20，最大 100）；响应 `{"list": [], "total": 0, "page": 1, "page_size": 20}`
- 认证：小程序端与后台均为 `Authorization: Bearer <JWT>`；H5 用 URL 内不可枚举 token（重新生成后旧链接立即失效）
- 后台 JWT payload 含 `admin_id`、`role`（super_admin / admin / cs）

## Data & Domain Rules（以 docs/ 为准，改动前先读）

- **选手唯一标识是系统生成的 `player_no`**（8 位字母数字），证件号不是主键
- **证件号不可逆哈希存储**（HMAC-SHA256 + 服务端密钥，不存明文），比对时重算哈希
- 手机号明文存储
- 积分/奖金规则挂在**赛事（events）**上，不挂赛季
- 机构归属**报名单（registrations）**，选手不同场次可代表不同机构
- 组别配置粒度 = 赛事 × 组别 × 舞种（`event_groups`），名额按此粒度
- **榜单用预计算快照表**（`player_points` / `institution_points`），成绩发布/纠错时重算，小程序端只读快照
- 资料字段直接挂在报名单上（证件照 png/jpg ≤10MB、音频 mp3 ≤100MB ≤4 分钟、节目名等），无审核流程
- 音频审核页：每赛事一个免登录 H5，按 `audio_review_token` 访问
- 舞种仅古典舞（`classical`）和民族民间舞（`folk`）；年龄组 U11/U13/U15 按出生年自动判定
- 支付：提交报名即创建订单并拉起微信支付；关闭/返回则订单取消消失但留档；小程序端无自助退款，需客服登记 + 管理员后台人工退款
- 一人同一赛事限报 1 个组别、可报多个舞种；不支持批量代报

## Backend Rules (Go)

- Handler 保持 thin：参数校验（binding + validator）→ 调用 service → 返回统一响应包装。
- 业务逻辑放 `internal/service/`，数据访问放 `internal/store/`；router/service/store 全部通过构造函数注入依赖，便于 httptest 与 fake 测试（参考 `internal/pkg/httpx/httpx_test.go`、`internal/store/migrate_test.go` 模式）。
- 所有需要取消/超时控制的工作必须接受 `context.Context`；`server.Run` 用 ctx 驱动优雅关闭。
- 错误用 `fmt.Errorf("...: %w", err)` 包装；用户可见错误信息清晰，细节用 zap 记录。业务代码不 panic。
- 使用 `gofmt`；代码注释与标识符用英文，用户可见文案可用中文。

## Frontend Rules (React / TypeScript)

- TypeScript strict，不用 `any`；函数组件 + hooks。
- 后台管理端与 H5 使用 React（工程在 `dance-admin/`，Ant Design Pro / Umi Max，非 Vite）；构建产物如何由后端托管在 T32 确定（embed 指令与被嵌入资源须同目录）。
- 请求封装统一处理响应包装（`code`/`message`/`data`）、Bearer token 与错误码提示；分页遵循统一分页结构。
- 不在前端存放业务规则副本（年龄判定、名额校验等以服务端为准）。

## Forbidden

- 不绕过 `service` 层在 handler 里直接写数据访问逻辑；不绕过 `store` 层拼 SQL。
- 不存储证件号明文、不把 JWT secret / 微信支付密钥等提交入库（密钥只经环境变量注入，`.env` 与 `data/` 不入库）。
- 不手改前端构建生成的产物（改 `dance-admin/` 源码后重新构建）。
- 不做旧 JSON 数据迁移（docs 决策 #10），`data/` 下旧数据仅作历史参考。
- 不新增与已锁定技术栈重复的主要依赖（不使用 echo/chi、不使用 sqlx/raw sql 替代 GORM、不引入其他 ORM）。

## Current State Notes

- v1 榜单（JSON 文件存储、原生 JS 音频页）代码已全部下线（2026-11-10），代码库为全新起点；原 `web/` 目录已删除，音频审核 H5 页以 React 形式落入 `dance-admin/src/pages/audio-review/`（路由 `/audio-review/:token`，数据接口 `GET /h5/audio-review/{token}`）。
- 现有基础：SQLite 数据层（`store/db.go` WAL 单连接池、`migrate.go` 版本化迁移、`tx.go` 事务）、`pkg/httpx` 统一响应与错误码、`middleware`（RequestID/AccessLog/Recovery）、`server` 优雅关闭。测试基线：`go test ./...` 全绿。
- 业务代码（model/service/各端 handler）按开发计划 T2 起落地，模型以 `docs/数据库表结构设计.md` v3 为准。
