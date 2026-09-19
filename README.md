# VTE - Multi-backend LLM API Gateway

[English](README.md) | [简体中文](README.zh-CN.md)

A lightweight, self-hosted API gateway that unifies multiple AI service providers with an OpenAI-compatible interface.

## ✨ Features

- 🔌 **Multi-backend Support** - Add any OpenAI-compatible API (OpenAI, Claude, Gemini, Ollama, etc.)
- 🎯 **Model Management** - Fetch models from providers and selectively enable them
- 🔑 **Unified Entry** - One URL + API Key for all your AI services
- 🖥️ **Web Admin Panel** - Beautiful web interface for easy management
- 📋 **Real-time Logs** - Terminal-style logging for debugging
- 🔄 **Stream Control** - Force streaming or non-streaming mode globally
- 🏷️ **Model Prefixes** - Organize models by provider with custom prefixes
- ✏️ **Model Aliases** - Custom display names for models (shows B to users, uses A internally)
- 📊 **Token Statistics** - Track daily token usage with 20-minute granular breakdown and request counts
- 🔐 **Secure** - Built-in authentication and API key management
- ⚡ **Lightweight** - Built with Go, ultra-low memory usage (~10-20MB)

---

## 🚀 Docker Deployment (Recommended)

**Simplest way:**
```bash
docker run -d \
  --name vte \
  -p 8050:8050 \
  -v vte-data:/app/data \
  --restart unless-stopped \
  rtyedfty/vte
```

Then visit http://YOUR_IP:8050, username: `admin`; see `docker logs vte` for the random initial password when `ADMIN_PASSWORD` is unset

**Custom port and password:**
```bash
docker run -d \
  --name vte \
  -p 80:8050 \
  -v vte-data:/app/data \
  -e ADMIN_PASSWORD=mypassword123 \
  --restart unless-stopped \
  rtyedfty/vte
```

**Parameters:**
| Parameter | Description | Required |
|-----------|-------------|----------|
| `-p 8050:8050` | Port mapping, change left number for different port | Yes |
| `-v vte-data:/app/data` | Data persistence | Recommended |
| `-e ADMIN_PASSWORD=xxx` | Custom admin password | Optional |
| `-e SECRET_KEY=xxx` | JWT secret key | Optional |
| `--restart unless-stopped` | Auto restart | Recommended |

**Using docker-compose:**

Create `docker-compose.yml`:
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

Run:
```bash
docker-compose up -d
```

**Update to latest version:**
```bash
# Pull latest image
docker pull rtyedfty/vte:latest

# Stop and remove old container
docker stop vte && docker rm vte

# Start new container (data will be preserved)
docker run -d --name vte -p 8050:8050 -v vte-data:/app/data --restart unless-stopped rtyedfty/vte:latest
```

Or use the update script:
```bash
# Linux/Mac
chmod +x update.sh && ./update.sh

# Windows
update.bat
```

---

## 💻 Local Deployment

### Prerequisites

- Go 1.21+ ([Download](https://go.dev/dl/))
- Node.js 18+ ([Download](https://nodejs.org/))

### Quick Start

**Windows:**
```cmd
start.bat
```

**Linux/Mac:**
```bash
chmod +x start.sh && ./start.sh
```

### Manual Build

```bash
# 1. Clone the repository
git clone https://github.com/starared/vte.git
cd vte

# 2. Build frontend
cd frontend
npm install
npm run build
cd ..

# 3. Build backend
cd backend
go mod tidy
go build -o vte .    # Linux/Mac
go build -o vte.exe .  # Windows
cd ..

# 4. Run
cd backend
./vte              # Linux/Mac
.\vte.exe          # Windows
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8050` |
| `HOST` | Bind address | `0.0.0.0` |
| `ADMIN_PASSWORD` | Initial password for a new administrator only | Random at first startup |
| `SECRET_KEY` | JWT secret | Auto-generated |
| `DATABASE_PATH` | SQLite path | `./data/gateway.db` |

Example:
```bash
# Linux/Mac
export ADMIN_PASSWORD=mypassword
./vte

# Windows
set ADMIN_PASSWORD=mypassword
.\vte.exe
```

### Performance Comparison

| Metric | Python | Go |
|--------|--------|-----|
| Memory | ~80-120MB | ~10-20MB |
| Startup | ~2-3s | <100ms |
| Binary | Requires Python | Single file |

---

## 📖 Quick Start Guide

### 1. Access Web Interface
Visit http://127.0.0.1:8050 and login with default credentials:
- Username: `admin`
- Password: random initial password in startup logs, or the `ADMIN_PASSWORD` supplied on first startup

### 2. Add a Provider
- Click "Add Provider" button
- Choose provider type (Standard OpenAI Compatible / Vertex Express)
- Fill in provider details:
  - **Name**: Display name (e.g., OpenAI, Claude)
  - **Model Prefix**: Optional prefix for model names (e.g., `openai`, `claude`)
  - **API URL**: Provider's API endpoint (must include `/v1`)
  - **API Key**: Your provider's API key

### 3. Fetch Models
- Click "Fetch Models" button for the provider
- Models will be automatically imported
- Enable the models you want to use

### 4. Use the API
Copy your API Key from Dashboard or Settings, then configure your client:

**API Endpoint**: `http://127.0.0.1:8050/v1`

**Example with curl**:
```bash
curl http://127.0.0.1:8050/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**Example with Python**:
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://127.0.0.1:8050/v1",
    api_key="YOUR_API_KEY"
)

response = client.chat.completions.create(
    model="gpt-4",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)
```

---

## ⚙️ Advanced Features

### Stream Mode Control
Go to Settings → Stream Mode to control streaming behavior:
- **Auto**: Follow client's request (default)
- **Force Stream**: All requests use streaming
- **Force Non-Stream**: All requests use non-streaming

### Model Prefixes
Add prefixes to organize models by provider:
- Set prefix when creating/editing provider (e.g., `openai`, `claude`)
- Models will be displayed as `prefix/model-name`
- Helps identify which provider a model belongs to

### Model Synchronization
Click "Fetch Models" to:
- Add new models from provider
- Update model display names (if prefix changed)
- Remove models that are no longer available

---

## 🔧 Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ADMIN_PASSWORD` | Initial password for a new administrator only | Random at first startup |
| `SECRET_KEY` | JWT secret key for authentication | Auto-generated |
| `DATABASE_PATH` | SQLite database file path | `./data/gateway.db` |

### Docker Volumes

Mount `/app/data` to persist:
- Database (user accounts, providers, models)
- Configuration settings

---

## 🌐 Supported Providers

VTE works with any OpenAI-compatible API. Here are some examples:

| Provider | Type | API URL | Notes |
|----------|------|---------|-------|
| OpenAI | Standard | `https://api.openai.com/v1` | Official OpenAI API |
| Anthropic Claude | Standard | OpenAI-compatible adapter endpoint | Native Messages API is not supported |
| Google Gemini | Vertex Express | N/A | Requires project ID |
| Ollama | Standard | `http://localhost:11434/v1` | Local models |
| Azure OpenAI | Standard | `https://{resource}.openai.azure.com/v1` | Azure endpoint |
| Any OpenAI-compatible | Standard | Custom URL | Self-hosted or third-party |

---

## 🛠️ Development

### Prerequisites
- Go 1.21+
- Node.js 18+
- npm or yarn

### Setup
```bash
# Clone repository
git clone https://github.com/starared/vte.git
cd vte

# Build frontend
cd frontend
npm install
npm run build

# Build backend
cd ../backend
go mod tidy
go build -o vte .

# Run
./vte
```

### Project Structure
```
vte/
├── backend/
│   ├── internal/
│   │   ├── auth/        # Authentication
│   │   ├── config/      # Configuration
│   │   ├── database/    # Database layer
│   │   ├── handlers/    # HTTP handlers
│   │   ├── models/      # Data models
│   │   ├── proxy/       # API proxy
│   │   └── router/      # Router setup
│   ├── data/            # SQLite database
│   ├── main.go          # Entry point
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── views/       # Vue pages
│   │   ├── api/         # API client
│   │   └── stores/      # State management
│   └── package.json
└── Dockerfile
```

---

## 📝 Changelog

### v1.0.10
- Fix key rotation, quota transactions, cancellation, limits and stream conversion. See [CHANGELOG.md](CHANGELOG.md) for migration notes.
- See [Nginx example](deploy/nginx.conf.example). Compose binds to loopback by default.
- Configure `UPSTREAM_TIMEOUT_SECONDS` (default 300) and `INCLUDE_STREAM_USAGE` (default true) as needed.

### v1.0.5
- ✅ **Multi API Key Support** - Add multiple API keys per provider with round-robin rotation
- ✅ **Connection Test** - Test provider connectivity with specific model and API key
- ✅ **Key Management** - Manage API keys directly in provider edit dialog
- ✅ **Usage Statistics** - Track usage count and last used time for each API key

### Previous Versions
- ✅ CORS support for cross-origin requests
- ✅ WebSocket support (`/v1/chat/completions/ws`)
- ✅ Auto-sync model prefixes when switching to Models page
- ✅ Stream mode control (auto/force-stream/force-non-stream)
- ✅ Model prefix support for better organization
- ✅ Vertex Express support
- ✅ API Key visibility toggle
- ✅ Real-time logs viewer

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

---

## 📄 License

MIT License - see LICENSE file for details

---

## 🔗 Links

- GitHub: [starared/vte](https://github.com/starared/vte)
- Docker Hub: [rtyedfty/vte](https://hub.docker.com/r/rtyedfty/vte)

---

## ⚠️ Security Notes

- Change default admin password immediately after first login
- Use HTTPS in production (reverse proxy recommended)
- Keep your API keys secure
- Regularly backup your database (`/app/data/gateway.db`)
