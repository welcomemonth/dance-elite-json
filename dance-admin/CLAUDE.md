# CLAUDE.md

## Project

优舞盟（dance-admin）— 少儿中国舞赛事系统的 PC 后台管理端 + 音频审核 H5。
基于 Ant Design Pro（Umi Max v4, antd v6, ProComponents v3），业务页面已替换为优舞盟 22 页结构
（数据看板/赛事/订单/报名运营/榜单中心/用户与内容/系统管理，菜单见 `config/routes.ts`）。
接口契约以 `../docs/优舞盟报名功能/API接口设计.md` 第二部分（`/api/v1/admin`）为准。

### 分层约定（新代码必须遵循）

- **页面** `src/pages/<module>/<page>/`：PageContainer + ProTable/ProForm，只调 service 层。
- **服务** `src/services/youwumeng/`：`typings.ts`（全部 DTO）、`<resource>.ts`（request 封装，路径=真实后端路径）、
  `labels.ts`（枚举→中文，前端不存业务规则）、`format.ts`（分→元等）。榜单中心接口在 API 文档中标注 *待定*，以 typings 契约为准。
- **Mock** `mock/youwumeng/`：express 处理器，路径与真实后端一一对应；共享内存数据集在 `_db.ts`，
  统一响应用 `_utils.ts` 的 `ok/fail/paginate`。新增接口要同时加 service + mock。
- **请求层**：`src/requestErrorConfig.ts` 在响应拦截器里按 `{code,message,data}` 约定抛出业务错误（code!==0）
  并解包 `data`，因此 service 声明的返回类型即业务数据本身。
- **数据加载**：页面统一用 `src/services/youwumeng/useService.ts` 的 `useService(service, options?)`
  （包装 umi `useRequest`，解决其 `{data:T}` 约定与解包后 service 的类型冲突）；支持 `manual/ready/refreshDeps/onSuccess/onError`。
  不要直接对 service 函数调 umi `useRequest`。

### Mock ↔ 真实后端切换（改一处即可）

- Mock 开发：`pnpm start`（默认，mock 拦截 `/api/**` 与 `/h5/**`）。
- 真实后端：`pnpm dev`（MOCK=none）+ `config/proxy.ts` 的 dev target 指向 Go 服务（默认 http://localhost:8080）。
- 测试账号：`admin/Admin@12345`（超管）、`ops/Ops@12345`（管理员）、`cs01/Cs@12345`（客服只读）。
- 模拟故障内置：退款工单 903 同意→41014 退款失败；非待处理工单操作→41013；cs 写操作→40301。

## Commands

`pnpm start` (dev+mock), `pnpm dev` (no mock → proxy 到真实后端), `pnpm build` (utoopack), `pnpm lint` (Biome+tsc), `pnpm test` (Jest), `npx antd lint ./src` (antd-specific checks).

Other: `pnpm openapi` (regenerate `src/services/`), `pnpm simple` (**irreversible** — commit first), `pnpm biome` (auto-fix), `pnpm tsc` (type-check only).

## ProComponents v3 / antd v6 适配要点（与 v2 习惯不同，易踩坑）

- 列配置「查询表单中隐藏」用 `search: false`（v3 已移除 `hideInSearch`）；「仅查询不显示」用 `hideInTable: true`。
- `ProDescriptions` 无 `.Item` 子组件：纯展示用 antd `Descriptions`（仍支持 `Descriptions.Item`）或 `items`。
- React 19 类型下 `useRef` 必须给初值：`useRef<ActionType>(null)`。
- antd v6：`Alert` 的 `message` 属性更名为 `title`；`Space direction` / `Drawer width` / `List` 标记 deprecated（警告级，暂未迁移）。

## Critical Rules

- **Never edit `src/services/ant-design-pro/`** — auto-generated, regenerate with `pnpm openapi`
- **Biome only** — no ESLint, no Prettier. Both `pnpm lint` and `npx antd lint ./src` must pass before commit
- **Always `npx antd info <Component>` before writing antd code** — don't guess APIs from memory
- **`pnpm simple` is irreversible** — always commit/branch first
- **Conventional commits** required (commitlint enforced)
- **TypeScript strict** · **Node ≥ 22** · **pnpm only** — lockfile is `pnpm-lock.yaml` (`package-lock.json` removed); build-script whitelist lives in `pnpm-workspace.yaml` (`allowBuilds`)
- **`.umi` dir is auto-generated** — delete `src/.umi` and restart if dev server acts up

## Architecture Essentials

**Config**: `config/config.ts` (defineConfig), `config/routes.ts` (declarative routes). Route `name` → `menu.xxx` i18n key; `access` field gates visibility.

**Convention files** (`src/`): `app.tsx` (runtime config + `getInitialState`), `access.ts` (permissions), `global.tsx` (side effects), `loading.tsx`, `typings.d.ts`.

**Auth**: `getInitialState()` → `GET /api/currentUser`; 401 → redirect login. `access.ts`: `canAdmin = currentUser.access === 'admin'`. Mock creds: `admin`/`ant.design` or `user`/`ant.design`.

**State**: `useModel('filename')` for global hooks (`src/models/`). `useModel('@@initialState')` for currentUser/settings. ProTable `request` prop for most data loading. `@tanstack/react-query` for complex server state.

**Styling priority**: Tailwind CSS v4 (layout) → antd-style v4 / `createStyles` (theme tokens) → CSS Modules → Less (legacy only).

**Request**: built-in `request` from `@umijs/max`, configured in `src/requestErrorConfig.ts`. Per-page `service.ts` for non-generated APIs.

**i18n**: 8 locales in `src/locales/`. `useIntl().formatMessage({ id, defaultMessage })`.

**Mock**: `mock/` (global) + `src/pages/**/_mock.ts` (co-located). Express-style handlers.

**Cloudflare Worker**: `cloudflare-worker/` — separate Hono app, own `package.json`, not an npm workspace.

## AI Skills

This project ships with two built-in Claude Code Skills (`.claude/skills/`). If you already have these skills in your project, no installation is needed — just run them directly. To update to the latest skill definitions, run `npx skills add ant-design/ant-design-pro`.

### `/pro-upgrade` — Project Upgrade

Run `/pro-upgrade` in Claude Code to auto-upgrade the project to the latest Ant Design Pro version. It diffs the latest template against this project and merges framework changes while preserving business code. Works for any version gap (v5→v6, v6.x→latest, etc.).

### `/antd` — Ant Design CLI

Run `/antd` in Claude Code for any antd-related work. It provides access to `@ant-design/cli` with offline metadata for antd v3/v4/v5/v6. Key commands:

- `npx antd info <Component>` — look up props/API before writing code (mandatory)
- `npx antd lint ./src` — check for deprecated or problematic usage (must pass before commit)
- `npx antd demo <Component> <demo>` — get working code examples
- `npx antd migrate <from> <to>` — migration checklist between major versions

## Page Co-location

Each page dir: `index.tsx`, optional `service.ts`, `_mock.ts`, `data.d.ts`, style files. Keep page-specific code with the page.

# CLAUDE.md

Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.

---