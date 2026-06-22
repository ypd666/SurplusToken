# SurplusToken

AI API Gateway — 基于 CPA + New API 的统一 AI 网关平台。

## 架构

```
┌─────────────────────────────────────────┐
│          New API (管理 + Web UI)          │
│  • 用户注册 / 登录                       │
│  • API Key 分发                          │
│  • 分组 + 配额管理                       │
│  • 计费 + 支付                           │
│  • OAuth 账号管理（代理 CPA）            │
│  • Web 管理后台                          │
│     └─ /oauth-accounts (新增)            │
└──────────────┬──────────────────────────┘
               │ HTTP 内部调用
┌──────────────▼──────────────────────────┐
│          CPA (AI 网关引擎)               │
│  • OAuth 账号管理 (Claude/Codex/Gemini)  │
│  • Token 自动刷新                        │
│  • 协议转换 (OpenAI ↔ Claude ↔ Gemini)   │
│  • 上游调度 (Round-Robin + Cooldown)     │
│  • 国产模型透传 (DeepSeek/GLM/Kimi...)   │
└──────────────┬──────────────────────────┘
               │ OAuth / API Key
┌──────────────▼──────────────────────────┐
│   Codex / Claude / Gemini / DeepSeek    │
│   GLM / Kimi / Qwen / Grok / ...        │
└─────────────────────────────────────────┘
```

## 目录结构

```
surplustoken/
├── config/
│   └── cpa-config.yaml        # CPA 网关配置
├── deploy/
│   ├── docker-compose.yml     # 统一部署编排
│   ├── Dockerfile.new-api     # New API 构建（含自定义代码）
│   └── .env.example           # 环境变量模板
├── new-api/                   # New API 源码（已修改）
│   ├── controller/
│   │   └── oauth_proxy.go     # 新增：CPA OAuth 代理
│   ├── router/
│   │   └── api-router.go      # 修改：注册 OAuth 路由
│   └── web/default/src/
│       ├── features/
│       │   └── oauth-accounts/ # 新增：OAuth 管理页面
│       ├── routes/_authenticated/
│       │   └── oauth-accounts/ # 新增：路由
│       └── hooks/
│           └── use-sidebar-data.ts  # 修改：侧边栏导航
├── cpa/                        # CPA 源码（未修改，仅引用）
├── scripts/
│   └── deploy.sh              # Clab 部署脚本
└── README.md
```

## 部署

### 1. 配置环境变量

```bash
cd deploy
cp .env.example .env
nano .env  # 修改所有密码和密钥
```

### 2. 部署到 Clab

```bash
./scripts/deploy.sh root@your-server-ip
```

### 3. 在 Clab 上启动

```bash
ssh root@your-server-ip
cd /opt/surplustoken/deploy
nano .env  # 确认配置
docker compose up -d --build
```

### 4. 初始化

1. 打开 `http://your-server-ip:3000`
2. 首次访问会进入 Setup 向导，创建 root 账号
3. 登录后进入 Channels 配置渠道（CPA 作为上游）
4. 进入 OAuth Accounts 页面连接上游 OAuth 账号

## CPA 渠道配置

在 New API 中添加 CPA 作为 OpenAI 兼容渠道：

| 参数 | 值 |
|------|-----|
| 类型 | AdvancedCustom / OpenAI Compatible |
| Base URL | `http://cpa:8317/v1` |
| API Key | `sk-surplustoken-gateway-internal` |
| 模型 | 填入你想暴露给用户的模型名称 |

## 支持的上游

| 类型 | 认证 | 备注 |
|------|------|------|
| Claude | OAuth (PKCE) | Claude Code 原生兼容 |
| Codex | OAuth (PKCE) | OpenAI Codex CLI |
| Gemini | OAuth | Google Gemini CLI |
| Antigravity | OAuth | 第三方中转 |
| DeepSeek | API Key | 在 CPA config 中配置 |
| GLM (智谱) | API Key | 在 CPA config 中配置 |
| Kimi (月之暗面) | API Key | 在 CPA config 中配置 |
| Qwen (通义千问) | API Key | 在 CPA config 中配置 |
| xAI Grok | OAuth | 在 CPA config 中配置 |
