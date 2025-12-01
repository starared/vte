# VTE 故障排除指南

本文档提供 Windows 系统上运行 VTE 时常见问题的解决方案。

## 目录

- [闪退问题](#闪退问题)
- [Windows 安全警告](#windows-安全警告)
- [环境配置问题](#环境配置问题)
- [端口占用问题](#端口占用问题)
- [依赖安装失败](#依赖安装失败)
- [构建失败](#构建失败)

---

## 闪退问题

### 症状
双击 `start.bat` 后，命令行窗口一闪而过，无法看到错误信息。

### 解决方案

#### 方法 1：使用诊断工具（推荐）
先运行 `diagnose.bat` 检查环境配置：
```cmd
diagnose.bat
```

#### 方法 2：在命令行中运行
1. 按 `Win + R`，输入 `cmd`，按回车打开命令提示符
2. 进入项目目录：
   ```cmd
   cd /d D:\path\to\vte
   ```
3. 运行启动脚本：
   ```cmd
   start.bat
   ```
4. 此时可以看到完整的错误信息

#### 方法 3：在 PowerShell 中运行
1. 右键点击开始菜单，选择"Windows PowerShell"
2. 进入项目目录：
   ```powershell
   cd D:\path\to\vte
   ```
3. 运行脚本：
   ```powershell
   .\start.bat
   ```

---

## Windows 安全警告

### 症状
运行时出现 "Windows 已保护你的电脑" 或防火墙警告。

### 解决方案

#### Windows Defender SmartScreen 警告
1. 点击 "更多信息"
2. 点击 "仍要运行"

#### 防火墙警告
首次启动时，Windows 防火墙可能会提示是否允许网络访问：
1. 勾选 "专用网络" 和 "公用网络"（根据需要）
2. 点击 "允许访问"

#### 杀毒软件误报
某些杀毒软件可能会误报 `vte.exe`。解决方法：
1. 将项目目录添加到杀毒软件的信任列表/排除列表
2. 或临时禁用实时防护后再运行

---

## 环境配置问题

### 症状
提示未安装 Go 或 Node.js。

### 解决方案

#### 安装 Go
1. 访问 [https://go.dev/dl/](https://go.dev/dl/)
2. 下载 Windows 版本（.msi 文件）
3. 运行安装程序，保持默认选项
4. 重新打开命令行窗口
5. 验证安装：
   ```cmd
   go version
   ```
   应显示类似 `go version go1.21.x windows/amd64`

#### 安装 Node.js
1. 访问 [https://nodejs.org/](https://nodejs.org/)
2. 下载 LTS 版本（推荐）
3. 运行安装程序，保持默认选项
4. 重新打开命令行窗口
5. 验证安装：
   ```cmd
   node --version
   npm --version
   ```

#### 环境变量问题
如果安装后仍提示找不到命令：
1. 按 `Win + R`，输入 `sysdm.cpl`，按回车
2. 点击 "高级" 选项卡
3. 点击 "环境变量"
4. 在 "系统变量" 中找到 "Path"
5. 确保包含 Go 和 Node.js 的安装路径：
   - Go: `C:\Program Files\Go\bin`
   - Node.js: `C:\Program Files\nodejs`
6. 重新打开命令行窗口

---

## 端口占用问题

### 症状
提示端口 8050 已被占用，或程序启动后立即退出。

### 解决方案

#### 查看占用端口的进程
```cmd
netstat -ano | findstr :8050
```
输出示例：
```
TCP    0.0.0.0:8050    0.0.0.0:0    LISTENING    12345
```
最后一列 (12345) 是进程 ID (PID)

#### 关闭占用端口的进程
```cmd
taskkill /PID 12345 /F
```
将 12345 替换为实际的 PID

#### 查看进程名称
```cmd
tasklist | findstr 12345
```

#### 修改 VTE 端口
如果不想关闭占用进程，可以修改 VTE 使用的端口：
1. 在启动时设置环境变量：
   ```cmd
   set PORT=8060
   start.bat
   ```

---

## 依赖安装失败

### 症状
npm install 或 go mod download 失败。

### 解决方案

#### npm 安装失败

##### 网络问题
使用国内镜像源：
```cmd
npm config set registry https://registry.npmmirror.com
```

##### 权限问题
以管理员身份运行命令提示符：
1. 按 `Win + X`
2. 选择 "Windows 终端（管理员）" 或 "命令提示符（管理员）"
3. 重新运行 `start.bat`

##### 依赖损坏
删除 node_modules 后重试：
```cmd
cd frontend
rmdir /s /q node_modules
npm install
```

#### Go 依赖下载失败

##### 网络问题
设置 Go 代理：
```cmd
go env -w GOPROXY=https://goproxy.cn,direct
```

##### 清理缓存后重试
```cmd
go clean -modcache
cd backend
go mod download
```

---

## 构建失败

### 症状
前端或后端构建出错。

### 解决方案

#### 前端构建失败

##### Node.js 版本过低
检查版本：
```cmd
node --version
```
需要 Node.js 18 或更高版本。如果版本过低，请重新安装最新 LTS 版本。

##### 重新安装依赖
```cmd
cd frontend
rmdir /s /q node_modules
del package-lock.json
npm install
npm run build
```

#### 后端构建失败

##### Go 版本过低
检查版本：
```cmd
go version
```
需要 Go 1.21 或更高版本。如果版本过低，请重新安装最新版本。

##### 重新下载依赖
```cmd
cd backend
go clean -modcache
go mod download
go mod tidy
go build -o vte.exe .
```

---

## 其他问题

### 数据库锁定
如果提示数据库被锁定：
1. 确保没有其他 VTE 实例在运行
2. 检查任务管理器中是否有 `vte.exe` 进程
3. 如有，结束该进程后重试

### 内存不足
构建过程中如果系统卡顿或失败：
1. 关闭其他占用内存的程序
2. 增加虚拟内存
3. 或使用 Docker 方式部署（推荐）

---

## 获取帮助

如果以上方法都无法解决问题：

1. **运行诊断工具**：运行 `diagnose.bat` 获取环境信息
2. **查看日志**：在命令行中运行 `start.bat`，复制完整的错误信息
3. **提交 Issue**：在 [GitHub Issues](https://github.com/starared/vte/issues) 提交问题，附上：
   - 操作系统版本
   - 诊断工具的输出
   - 完整的错误信息

---

## 相关链接

- [项目主页](https://github.com/starared/vte)
- [Docker 部署指南](README.zh-CN.md#-docker-部署推荐)
- [Go 官网](https://go.dev/)
- [Node.js 官网](https://nodejs.org/)
