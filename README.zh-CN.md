# VTE - 多后端 LLM API 网关

[English](README.md) | [简体中文](README.zh-CN.md)

轻量级、自托管的 API 网关，统一管理多个 AI 服务提供商，提供 OpenAI 兼容接口。

## ✨ 功能特性

- 🔌 **多后端支持** - 支持任意 OpenAI 兼容 API（OpenAI、Claude、Gemini、Ollama 等）
- 🎯 **模型管理** - 从提供商拉取模型，选择性启用
- 🔑 **统一入口** - 一个 URL + API Key 管理所有 AI 服务
- 🖥️ **Web 管理界面** - 美观的 Web 界面，轻松管理
- 🔄 **流式控制** - 全局强制流式或非流式模式
- 🏷️ **模型前缀** - 使用自定义前缀组织不同提供商的模型
- ✏️ **模型别名** - 自定义模型显示名称（用户看到B模型，实际使用A模型）
- 📊 **Token统计** - 追踪每日token消耗，每20分钟粒度显示使用量和请求次数
- 🔐 **安全可靠** - 内置身份验证和 API Key 管理

---

## 🚀 Docker 部署（推荐）

**最简单的方式：**
```bash
docker run -d \
  --name vte \
  -p 8050:8050 \
  -v vte-data:/app/data \
  --restart unless-stopped \
  rtyedfty/vte
```

然后访问 http://你的IP:8050，默认用户名 `admin`；未设置 `ADMIN_PASSWORD` 时，首次启动随机密码见 `docker logs vte`

**自定义端口和密码：**
```bash
docker run -d \
  --name vte \
  -p 80:8050 \
  -v vte-data:/app/data \
  -e ADMIN_PASSWORD=mypassword123 \
  --restart unless-stopped \
  rtyedfty/vte
```

**参数说明：**
| 参数 | 说明 | 是否必须 |
|------|------|----------|
| `-p 8050:8050` | 端口映射，改左边数字换端口 | 必须 |
| `-v vte-data:/app/data` | 数据持久化 | 建议 |
| `-e ADMIN_PASSWORD=xxx` | 自定义管理员密码 | 可选 |
| `-e SECRET_KEY=xxx` | JWT 密钥 | 可选 |
| `-e TZ=Asia/Shanghai` | 时区设置（默认北京时间） | 可选 |
| `--restart unless-stopped` | 自动重启 | 建议 |

**使用 docker-compose：**

创建 `docker-compose.yml`：
```yaml
version: '3.8'
services:
  vte:
    image: rtyedfty/vte
    ports:
      - "8050:8050"
    volumes:
      - vte-data:/app/data
    restart: unless-stopped

volumes:
  vte-data:
```

运行：
```bash
docker-compose up -d
```

**更新到最新版本：**

⚠️ **重要提示**：
1. 更新时必须使用 `-v vte-data:/app/data` 挂载数据卷
2. 更新前建议先在设置页面复制保存你的 API Key

使用更新脚本（推荐）：
```bash
# Linux/Mac
chmod +x update.sh && ./update.sh

# Windows
update.bat
```

或手动更新：
```bash
# 拉取最新镜像
docker pull rtyedfty/vte:latest

# 停止并删除旧容器
docker stop vte && docker rm vte

# 启动新容器（数据会保留）
docker run -d --name vte -p 8050:8050 -v vte-data:/app/data --restart unless-stopped rtyedfty/vte:latest
```

**检查数据卷是否正确挂载：**
```bash
docker inspect vte | grep vte-data
```
应该看到 `/app/data` 挂载到 `vte-data` 卷

---

## 💻 本地部署

**启动服务（自动构建）：**

Windows：
```cmd
start.bat
```

Linux/Mac：
```bash
chmod +x start.sh && ./start.sh
```

脚本会自动：
- 检查并安装依赖（Go、Node.js）
- 安装/更新前端依赖
- 构建前端和后端
- 启动服务

**停止服务：** 按 `Ctrl+C` 或直接关闭窗口

**手动构建：**
```bash
# 构建前端
cd frontend && npm install && npm run build

# 构建后端
cd ../backend && go mod tidy && go build -o vte .

# 运行
./vte
```

---

## 📖 快速开始

### 1. 访问 Web 界面
访问 http://127.0.0.1:8050 并使用管理员账号登录：
- 用户名：`admin`
- 密码：首次启动日志中的随机密码，或首次启动时设置的 `ADMIN_PASSWORD`

### 2. 添加提供商
- 点击"添加提供商"按钮
- 选择提供商类型（标准 OpenAI 兼容 / Vertex Express）
- 填写提供商信息：
  - **名称**：显示名称（如 OpenAI、Claude）
  - **模型前缀**：可选的模型名称前缀（如 `openai`、`claude`）
  - **API 地址**：提供商的 API 端点（必须包含 `/v1`）
  - **API Key**：提供商的 API 密钥

### 3. 拉取模型
- 点击提供商的"拉取模型"按钮
- 模型会自动导入
- 启用你想使用的模型

### 4. 使用 API
从仪表盘或设置页面复制你的 API Key，然后配置客户端：

**API 端点**：`http://127.0.0.1:8050/v1`

**curl 示例**：
```bash
curl http://127.0.0.1:8050/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "你好！"}]
  }'
```

**Python 示例**：
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8050/v1",
    api_key="YOUR_API_KEY"
)

response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "你好！"}]
)
print(response.choices[0].message.content)
```

---

## ⚙️ 高级功能

### 流式模式控制
进入设置 → 流式模式，控制流式行为：
- **自动**：跟随客户端请求（默认）
- **强制流式**：上游使用流式，返回格式仍遵循客户端请求
- **强制非流式**：上游使用非流式，返回格式仍遵循客户端请求

### 模型前缀
添加前缀来组织不同提供商的模型：
- 创建/编辑提供商时设置前缀（如 `openai`、`claude`）
- 模型会显示为 `前缀/模型名`
- 帮助识别模型属于哪个提供商

### 模型同步
点击"拉取模型"会：
- 添加提供商的新模型（默认不启用）
- 更新模型显示名称（如果前缀改变）
- 删除上游已不再列出的、通过拉取得到的模型
- 手动添加的模型保持不变；改过名字的模型（以及 v1.2.0 之前的旧模型）只停用不删除

### 密钥轮询与自动切换
一个提供商可以添加多个密钥，请求会轮流使用。如果上游以 401/403/429 拒绝了某个密钥，网关会立即换下一个密钥重试（不占用「最大重试次数」）。

### WebSocket
`/v1/chat/completions/ws` 支持通过 `Authorization` 请求头传递 API Key；浏览器无法设置请求头时，可以用子协议传递：`new WebSocket(url, ["bearer", "YOUR_API_KEY"])`。旧的 `?api_key=` 参数仍然可用，但密钥会出现在反向代理的访问日志里，不推荐。

---

## 🔧 配置说明

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `ADMIN_PASSWORD` | 仅初始化新管理员；不会重置已有密码 | 首次启动随机生成 |
| `SECRET_KEY` | JWT 认证密钥 | 自动生成 |
| `DATABASE_PATH` | SQLite 数据库文件路径 | `./data/gateway.db` |
| `MAX_REQUEST_BODY_MB` | 网关请求体大小上限（MB） | `32` |
| `UPSTREAM_TIMEOUT_SECONDS` | 上游最长无响应时间（秒）：等待响应头、以及流式输出中两段数据之间的间隔；输出总时长不受限制 | `300` |
| `INCLUDE_STREAM_USAGE` | 流式请求时自动向上游附加 `stream_options.include_usage` 获取 Token 用量；上游不支持该参数时设为 `false` | `true` |
| `TOKEN_STATS_TZ` | Token 统计每日重置所用时区 | `Asia/Shanghai` |
| `TOKEN_STATS_RESET_HOUR` | Token 统计每日重置的整点（0-23） | `15` |
| `TRUSTED_PROXIES` | 信任的反向代理地址（逗号分隔，支持 CIDR），只有来自这些地址的 `X-Forwarded-For` 才会被采信 | `127.0.0.1,::1` |

### 反向代理（Nginx 等）

VTE 用客户端 IP 做登录频率限制（每个 IP 每分钟 10 次）和日志记录。经过反向代理时，VTE 看到的来源是代理的地址，必须把代理加入 `TRUSTED_PROXIES` 才能拿到真实 IP，否则所有人会共用同一个登录额度。

- Nginx 和 VTE 都直接跑在宿主机上：默认值即可。
- VTE 跑在 Docker 里、Nginx 在宿主机上：请求来自 Docker 网关（如 `172.17.0.1`、`172.18.0.1`），设置 `TRUSTED_PROXIES=127.0.0.1,::1,172.16.0.0/12`。
- Nginx 也在 Docker 里：填写 Nginx 容器所在网络的网段。

只信任你自己的代理：被信任的地址可以伪造任意客户端 IP。

### Docker 数据卷

挂载 `/app/data` 以持久化：
- 数据库（用户账号、提供商、模型）
- 配置设置

---

## 🌐 支持的提供商

VTE 支持任何 OpenAI 兼容的 API。以下是一些示例：

| 提供商 | 类型 | API 地址 | 备注 |
|--------|------|----------|------|
| OpenAI | 标准 | `https://api.openai.com/v1` | 官方 OpenAI API |
| Anthropic Claude | 标准 | 第三方 OpenAI 兼容转换端点 | 不支持直接对接原生 Messages API |
| Google Gemini | Vertex Express | 无 | 需要项目 ID |
| Ollama | 标准 | `http://localhost:11434/v1` | 本地模型 |
| Azure OpenAI | 标准 | `https://{resource}.openai.azure.com/v1` | Azure 端点 |
| 任何兼容 API | 标准 | 自定义 URL | 自托管或第三方 |

---

## 🛠️ 开发指南

### 环境要求
- Go 1.26+
- Node.js 18+

### 安装步骤
```bash
# 克隆仓库
git clone https://github.com/starared/vte.git
cd vte

# 构建前端
cd frontend
npm install
npm run build

# 构建后端
cd ../backend
go mod tidy
go build -o vte .

# 运行
./vte
```

### 项目结构
```
vte/
├── backend/             # Go 后端
│   ├── internal/
│   │   ├── models/      # 数据模型
│   │   ├── handlers/    # API 处理器
│   │   ├── database/    # 数据库
│   │   └── router/      # 路由
│   ├── main.go          # 入口
│   └── go.mod
├── frontend/            # Vue 前端
│   ├── src/
│   │   ├── views/       # 页面
│   │   ├── api/         # API 客户端
│   │   └── stores/      # 状态管理
│   └── package.json
└── Dockerfile
```

---

## 📝 更新日志

### v1.2.1
- 安全：`golang-jwt/jwt` 升级到 5.3.1（修复 CVE-2025-30204，构造的 token 可在未登录状态下耗尽内存），其余 Go 依赖一并升级；改用 Go 1.26 构建；CI 增加 Dependabot 和 `govulncheck`。
- 网关：上游流式响应没有发送 `data: [DONE]` 但已给出 `finish_reason` 时视为完整响应（之前会多发一条错误、丢失 Token 统计，强制流式转换还会返回 502）；流中的上游错误信息原样传给客户端。
- 服务端：增加请求头读取超时和空闲连接超时；`index.html` 不缓存、带哈希的静态资源长期缓存，升级后不再出现白屏需要强刷。
- Token 估算对 gpt-4o / gpt-4.1 / gpt-5 / o 系列改用 `o200k_base` 词表。
- 清理：无用代码、过时的 `backend/Dockerfile`、Makefile 中的 CGO 参数；`package-lock.json` 改回官方 npm 源；文档更新。详见 [CHANGELOG.md](CHANGELOG.md)。

### v1.2.0
- 网关：上游返回 401/403/429 时自动切换到下一个密钥；流式输出不再在 5 分钟时被切断；已发到上游的请求不再自动重试（避免重复扣费）。
- 「拉取模型」不再删除手动添加和改过名字的模型。
- 修改用户名后不会被登出；修改密码后其他设备上的登录自动失效。
- 删除提供商时一并删除其密钥；Token 估算不再联网下载词表；统计重置时间可配置。
- 设置页恢复速率限制、并发限制和自定义限流规则。
- 模型显示名称不允许重复；改名或修改前缀时，临时 API 和限流规则里的引用自动更新；因上游下线被停用的模型重新上线后自动启用。
- 提供商支持编辑额外请求头；新密码至少 8 位；Docker 镜像改用 Go 1.24 / Node 22 构建。
- 前端修复（HTTP 下复制失败、错误提示重复、暗色模式等）与手机端适配。详见 [CHANGELOG.md](CHANGELOG.md)。

### v1.1.0
- 全新温暖中性界面主题（emerald 青绿强调色）：侧边栏、顶栏、登录页、卡片及暗色模式全面焕新。
- Token 统计：移除趋势曲线图（保留数字总览与模型明细表）。
- 移除「日志」页面及其导航入口。
- 修复 Token 统计页的内存泄漏（未移除的 resize 监听器）。

### v1.0.12
- 限制网关请求体大小（默认 32MB，可通过 `MAX_REQUEST_BODY_MB` 配置），防止超大请求体耗尽内存；超限返回 HTTP 413。
- 支持优雅关闭：收到 `SIGINT`/`SIGTERM`（如 `docker stop`）时，最多等待进行中的请求完成 30 秒后再退出，而非直接中断。
- 依赖安全升级（`gin`、`golang.org/x/net`、`golang.org/x/crypto`、`golang-jwt`、`gorilla/websocket`），保持 Go 1.21 兼容。详见 [CHANGELOG.md](CHANGELOG.md)。

### v1.0.11
- 修复轮询、事务锁、取消请求、限流和流式转换。完整变更及升级注意事项见 [CHANGELOG.md](CHANGELOG.md)。
- Nginx 反代参考 [deploy/nginx.conf.example](deploy/nginx.conf.example)。Compose 默认仅本机访问。
- `UPSTREAM_TIMEOUT_SECONDS` 可调整上游总超时；`INCLUDE_STREAM_USAGE=false` 可关闭自动添加 usage 参数。

### v1.0.5
- ✅ **多密钥支持** - 每个提供商支持添加多个 API Key，自动轮询使用
- ✅ **连接测试** - 测试提供商连接，可选择特定模型和密钥
- ✅ **密钥管理** - 在提供商编辑界面直接管理密钥
- ✅ **使用统计** - 记录每个密钥的使用次数和最后使用时间

### 历史版本
- ✅ CORS 跨域支持
- ✅ WebSocket 支持（`/v1/chat/completions/ws`）
- ✅ 切换到模型管理页面时自动同步前缀
- ✅ 流式模式控制（自动/强制流式/强制非流式）
- ✅ 模型前缀支持，更好地组织模型
- ✅ Vertex Express 支持
- ✅ API Key 显示/隐藏切换
- ✅ 实时日志查看器

---

## 🤝 贡献

欢迎贡献！请随时提交 Pull Request。

---

## 📄 许可证

MIT License - 详见 LICENSE 文件

---

## 🔗 相关链接

- GitHub：[starared/vte](https://github.com/starared/vte)
- Docker Hub：[rtyedfty/vte](https://hub.docker.com/r/rtyedfty/vte)

---

## ⚠️ 安全提示

- 首次登录后立即修改默认管理员密码
- 生产环境使用 HTTPS（建议使用反向代理）

---

## 🔧 故障排除

**容器启动后无法访问？**
```bash
# 1. 检查容器日志
docker logs vte

# 2. 如果没有日志输出，尝试重启容器
docker restart vte

# 3. 如果仍然无法访问，删除容器重新创建（数据会保留）
docker stop vte && docker rm vte
docker run -d --name vte -p 8050:8050 -v vte-data:/app/data --restart unless-stopped rtyedfty/vte:latest
```
- 妥善保管 API 密钥
- 定期备份数据库（`/app/data/gateway.db`）
