# 3GPP MCP Server

3GPP 协议规范知识库，为 AI Agent 提供 3GPP 协议知识查询能力。支持 MCP 协议和 CLI 两种访问方式。

详细设计见 [DESIGN.md](./DESIGN.md)。

## 安装

```bash
go build -o bin/3gpp-mcp ./cmd/3gpp-mcp/
```

纯 Go 实现，无外部二进制依赖（不需要 LibreOffice）。

## CLI 使用

所有命令默认输出人类可读文本，加 `--json` 输出结构化 JSON 供 AI 使用。

### 浏览规范目录

```bash
# 全部规范列表
3gpp-mcp catalog

# 指定系列
3gpp-mcp catalog --series 38

# 关键词搜索标题
3gpp-mcp catalog --keyword "RRC"
```

### 阅读规范内容

```bash
# 规范概览（标题、版本、章节目录）
3gpp-mcp spec 38.331

# 指定 Release 全文
3gpp-mcp spec 38.331 --release Rel-18

# 指定章节
3gpp-mcp spec 38.331 --release Rel-18 -s 5.3.7

# AI 可读 JSON
3gpp-mcp spec 38.331 --release Rel-18 -s 5.3.7 --json
```

### 规范内搜索

```bash
# 在指定规范中搜索关键词
3gpp-mcp search 38.331 "RRCReestablishment"

# 带上下文行数
3gpp-mcp search 38.331 "RRCReestablishment" -c 5
```

### 缓存管理

```bash
# 查看已缓存规范
3gpp-mcp cache-status

# 同步远端目录（刷新元数据）
3gpp-mcp sync

# 清除指定规范缓存
3gpp-mcp cache-clear 38.331

# 修复（重建数据库和索引）
3gpp-mcp repair
```

## MCP Server

### 配置 Claude Desktop / opencode

```json
{
  "mcpServers": {
    "3gpp": {
      "command": "/path/to/3gpp-mcp",
      "args": ["server", "--transport", "stdio"]
    }
  }
}
```

### SSE 模式（远程 Agent）

```bash
3gpp-mcp server --transport sse --addr :8080
```

### 同时启用 stdio + SSE

```bash
3gpp-mcp server --transport both --addr :8080
```

## MCP 资源

| URI | 说明 |
|---|---|
| `3gpp://catalog` | 全部已知规范列表 |
| `3gpp://catalog/{series}` | 指定系列下的规范 |
| `3gpp://specs/{spec_id}` | 规范概览：标题、版本、章节目录 |
| `3gpp://specs/{spec_id}/{release}` | 规范全文（透明触发下载） |
| `3gpp://specs/{spec_id}/{release}/{section}` | 指定章节内容 |

## MCP 工具

| 工具 | 参数 | 说明 |
|---|---|---|
| `list_specs` | `series?`, `keyword?` | 浏览规范目录（不触发下载） |
| `search_spec` | `spec_id`, `query`, `release?` | 在指定规范内搜索关键词（透明触发下载） |

## 数据来源

- **目录元数据**：启动时从 `3gpp.org/dynareport?code=status-report.htm` 抓取，包含规范编号、标题、版本、工作组
- **规范内容**：通过 FTP 匿名下载 ZIP 包，内含 `.docx` 文档，纯 Go 解析

## 工作原理

1. **目录始终可用**：首次启动从 dynareport 抓取全量规范目录，存入 SQLite
2. **规范按需获取**：首次查询某规范时，透明触发 FTP 下载 + .docx 解析 + 入库 + 索引
3. **缓存透明**：Agent 无需感知缓存状态。查询已缓存规范为毫秒级响应
