# Token消耗统计功能

## 功能说明

新增了完整的Token消耗统计功能，包括：

### 1. 数据记录
- 自动记录每次API调用的token使用情况
- 记录内容包括：模型名称、提供商、输入token、输出token、总token数
- 支持流式和非流式响应的token统计

### 2. 统计展示
- **今日总览**：显示今天的总token、输入token、输出token消耗
- **24小时趋势图**：以图表形式展示每小时的token消耗趋势
- **模型使用详情**：按模型分组显示详细的使用统计，包括请求次数和各类token消耗

### 3. 自动刷新
- 每天下午3点自动刷新统计（实际上是开始新一天的统计）
- 前端页面支持自动刷新（每30秒）
- 手动刷新按钮

### 4. 数据管理
- 每天下午3点刷新时自动清理昨天的数据，只保留当天统计
- 支持手动重置今日统计
- 数据持久化存储在SQLite数据库中

## 使用方法

### 直接运行（推荐）

**Windows:**
```bash
start.bat
```

**Linux/Mac:**
```bash
bash start.sh
```

脚本会自动：
1. 检查并安装依赖（Go、Node.js）
2. 安装前端依赖（包括新增的echarts）
3. 构建前端和后端
4. 启动服务

### 访问统计页面

启动后访问 http://127.0.0.1:8050，登录后在左侧菜单中点击"Token统计"即可查看。

## API接口

### 获取今日统计
```
GET /api/tokens/stats
```

返回示例：
```json
{
  "total_tokens": 15000,
  "prompt_tokens": 10000,
  "completion_tokens": 5000,
  "hourly_stats": [
    {"hour": 0, "total_tokens": 0},
    {"hour": 1, "total_tokens": 500},
    ...
  ],
  "model_stats": [
    {
      "model_name": "gpt-4",
      "provider_name": "OpenAI",
      "total_tokens": 8000,
      "prompt_tokens": 5000,
      "completion_tokens": 3000,
      "request_count": 10
    }
  ]
}
```

### 重置今日统计
```
DELETE /api/tokens/stats
```

## 技术实现

### 后端
- 新增 `token_usage` 数据库表
- 新增 `token_stats.go` handler处理统计查询
- 新增 `scheduler.go` 定时任务模块
- 修改 `openai.go` 在响应中提取并记录token使用

### 前端
- 新增 `TokenStats.vue` 页面组件
- 使用 ECharts 绘制趋势图
- 使用 Element Plus 表格展示详细数据

## 注意事项

1. 只有在API响应中包含 `usage` 字段时才会记录token统计
2. 流式响应需要提供商返回usage信息才能统计
3. 每天下午3点的"刷新"实际上是逻辑上的分界点，历史数据仍然保留
4. 数据库会自动清理30天前的记录以节省空间
