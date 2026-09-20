# VTE - 多后端 LLM API 网关

[English](README.md) | [简体中文](README.zh-CN.md)

轻量级、自托管的 API 网关，统一管理多个 AI 服务提供商，提供 OpenAI 兼容接口。

## ✨ 功能特性

- 🔌 **多后端支持** - 支持任意 OpenAI 兼容 API（OpenAI、Claude、Gemini、Ollama 等）
- 🎯 **模型管理** - 从提供商拉取模型，选择性启用
- 🔑 **统一入口** - 一个 URL + API Key 管理所有 AI 服务
- 🖥️ **Web 管理界面** - 美观的 Web 界面，轻松管理
- 📋 **实时日志** - 终端风格日志，方便调试
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
- 添加提供商的新模型
- 更新模型显示名称（如果前缀改变）
- 删除已下线的模型

---

## 🔧 配置说明

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `ADMIN_PASSWORD` | 仅初始化新管理员；不会重置已有密码 | 首次启动随机生成 |
| `SECRET_KEY` | JWT 认证密钥 | 自动生成 |
| `DATABASE_PATH` | SQLite 数据库文件路径 | `./data/gateway.db` |

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
- Go 1.21+
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

### v1.0.10
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
