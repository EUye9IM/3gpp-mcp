# 3GPP MCP Server 设计文档

## 一、项目定位

为 AI Agent 提供 3GPP 协议规范知识查询的 MCP 服务器。支持：
- **MCP 协议**（JSON-RPC over stdio / SSE）
- **CLI 命令行**（人类可读文本 + AI 可读 `--json`）

Agent 通过 URI 读取规范资源、调用工具搜索协议细节，服务端透明处理缓存和下载。

## 二、架构概览

```
cmd/3gpp-mcp/main.go          # cobra 入口
internal/
  cli/                        # CLI 子命令（catalog, spec, search, server, mgmt）
  mcp/                        # MCP 协议：server, handler, transports (stdio/SSE)
  core/                       # 核心业务逻辑（cli 与 mcp 共享）
  model/                      # 领域类型：Spec, Section, SearchResult
  store/                      # 数据层：SQLite + bleve 索引
  ingest/                     # 按需下载 + .docx 解析流水线
  config/                     # 配置结构体与默认值
```

### 依赖方向

```
cli/ ──▶ core/ ◀── mcp/
           │
           ▼
          store/ ◀── ingest/
           │
           ▼
         model/
```

- `cli/` 和 `mcp/` 只调用 `core/`，不触碰 `store/`
- `core/` 调 `store/`（查询），缺数据时调 `ingest/` 触发下载解析
- `ingest/` 写回 `store/`，不调 `core/`
- `model/` 被所有层引用

## 三、技术约束

| 项 | 值 |
|---|---|
| Go 版本 | 1.22 |
| CGO | 禁止 |
| 外部二进制依赖 | 无（不用 LibreOffice） |
| SQLite 驱动 | `modernc.org/sqlite`（纯 Go） |
| 搜索 | `github.com/blevesearch/bleve/v2`（per-spec 索引） |
| CLI | `github.com/spf13/cobra` |
| MCP | `github.com/mark3labs/mcp-go` |
| FTP | `github.com/jlaffaye/ftp` |
| .docx 解析 | Go 标准库 `archive/zip` + `encoding/xml` |

## 四、数据模型

### 4.1 规范类型

```go
type Spec struct {
    ID      string // "38.331"
    Title   string // "NR; Radio Resource Control (RRC); Protocol specification"
    Series  string // "38"
    WG      string // "R2"
    Version string // "19.3.0" (dynareport 报告的最新版本号)
}
```

### 4.2 章节类型

```go
type Section struct {
    SpecID        string // "38.331"
    Release       string // "Rel-18"
    SectionNumber string // "5.3.7"
    ParentNumber  string // "5.3" (根章节为 NULL)
    Title         string // "RRC Reestablishment"
    Content       string // 正文（包含父节的引导段落）
}
```

SQLite 表结构：

```sql
CREATE TABLE sections (
    id              INTEGER PRIMARY KEY,
    spec_id         TEXT NOT NULL,
    release         TEXT NOT NULL,
    section_number  TEXT NOT NULL,    -- "5.3.7"
    parent_number   TEXT,             -- "5.3" (NULL for top-level)
    title           TEXT NOT NULL,
    content         TEXT NOT NULL,
    UNIQUE(spec_id, release, section_number)
);

CREATE INDEX idx_sections_parent ON sections(spec_id, release, parent_number);
```

物化路径，parent 按最后一个 `.` 拆分：

- "5.3" → parent IS NULL
- "5.3.7" → parent = "5.3"
- "5.3.7.2" → parent = "5.3.7"
- "A.1" → parent = "A"（Annex 用字母作根）

### 4.3 搜索结果类型

```go
type SearchResult struct {
    SpecID        string
    Release       string
    SectionNumber string
    SectionTitle  string
    Content       string
}
```

**没有** Procedure、IE、Compare 类型。所有语义查询统一通过全文搜索完成。

## 五、数据获取策略

### 5.1 目录元数据（启动时）

**数据源**：`https://www.3gpp.org/dynareport?code=status-report.htm`

```html
<!-- HTML 结构（data 可被直接抓取） -->
<tr class="even/odd">
  <td>TS</td>
  <td><a href="/DynaReport/38331.htm">38.331</a></td>
  <td>NR; Radio Resource Control (RRC); Protocol specification</td>
  <td>19.3.0</td>
  <td>R2</td>
</tr>
```

**行为**：
- 首次启动 DB 为空时 → 自动抓取建目录
- `sync` 命令 → 手动触发重新抓取
- 若 DB 损坏（`PRAGMA integrity_check` 失败）→ 删除重建 + 重新抓取

### 5.2 规范内容（按需下载）

**数据源**：FTP 匿名下载 ZIP 包

- FTP 主机：`www.3gpp.org`
- 路径格式：`/Specs/archive/{series}_series/{spec_id}/`（全版本历史）或 `/Specs/latest/Rel-N/{series}_series/`（指定 Release 最新版）
- 文件名格式：`{spec_id_compact}{version_suffix}.zip`（如 `38331-g70.zip`）

**ZIP 内容**：`.docx` 文件（Office Open XML 格式），本质是 ZIP 套 XML。

**解析策略**：纯 Go 标准库解析 `word/document.xml`：
1. 提取所有 `<w:p>` 段落，附带 `<w:pPr>/<w:pStyle>` 样式标签
2. 样式 `Heading1`~`Heading5` 标识章节层级
3. 样式 `Heading8` 标识 Annex（附录，用字母编号）
4. 章节号从**文本 run（`<w:r>`）边界**提取：第一个匹配编号格式的 run 为章节号，其余 run 拼接为标题
5. 章节号空格归一化：`"4. 2 .1"` → `"4.2.1"`
6. 按样式层级构建 section tree，推算 parent_number

### 5.3 流水线（download → store → index）

每次下载在独立临时目录中进行。任意步骤失败时删除临时目录、清除该 spec/release 的部分 DB 条目，返回错误让调用方重试。

## 六、透明懒加载

Agent 视角：读取 `3gpp://specs/38.331/Rel-18/5.3.7` 即可获得内容。Agent **不知道**该规范是否已缓存。

### 核心流程

```go
var ErrNotCached = errors.New("spec not in local cache")

// core/spec.go
func (c *Core) GetSpecContent(specID, release, sectionNumber string) (*model.Section, error) {
    result, err := c.specStore.GetContent(specID, release, sectionNumber)
    if err == nil {
        return result, nil
    }
    if errors.Is(err, store.ErrNotCached) {
        // 透明触发下载 → 解析 → 入库
        if err := c.pipeline.Run(specID, release); err != nil {
            return nil, fmt.Errorf("ingest: %w", err)
        }
        // 重查
        return c.specStore.GetContent(specID, release, sectionNumber)
    }
    // 其他错误（DB 故障等）→ 透传
    return nil, err
}
```

### 错误区分

| store 返回 | core 行为 |
|---|---|
| `nil` | 直接返回 |
| `ErrNotCached` | 触发 ingest → 重查 |
| 其他 `error` | 透传，不重试 |

## 七、并发控制

- **单进程内**：`sync.Mutex` 按 `(specID, release)` 做去重。获取锁后二次检查（double-checked locking），防止等锁期间其他 goroutine 已完成下载
- **跨进程**：不处理（接受偶尔重复下载，SQLite `UNIQUE` 约束兜底）
- **SQLite WAL 模式**：允许多读一写

## 八、搜索

### 8.1 索引

- **Per-spec per-release bleve 索引**：路径 `data/index/{spec_id}/{release}/`
- 首次下载规范时懒创建
- 索引粒度：每个 `<w:p>` 段落为一个 bleve 文档

### 8.2 搜索范围

- 仅支持 `search_spec(spec_id, query, release?)` — 在指定规范内搜索
- 不提供跨规范内容搜索
- `list_specs --keyword "RRC"` 搜索的是**标题**，非内容

### 8.3 缓存清除

`cache-clear <spec_id>` 清除：
1. SQLite 中该 spec 相关行
2. 对应的 bleve 索引目录
3. 缓存的 ZIP 文件

## 九、Release → 版本映射

- `--release Rel-18` → 取 dynareport 报告的该系列最新版本号
- 例如 spec 38.331 在 dynareport 中版本为 18.7.0，则 `Rel-18` 映射到 18.7.0
- v1 不提供精确版本号选择

## 十、CLI 命令

```bash
3gpp-mcp catalog [--series X] [--keyword X]
3gpp-mcp spec <spec_id> [--release X] [-s section]
3gpp-mcp search <spec_id> <query> [-c context_lines] [--release X]
3gpp-mcp server [--transport stdio|sse|both] [--addr :8080]
3gpp-mcp sync
3gpp-mcp cache-status
3gpp-mcp cache-clear <spec_id>
3gpp-mcp repair
```

- 所有命令支持 `--json` 切换为结构化 JSON 输出
- 默认输出为 ANSI 着色人类可读文本

## 十一、MCP 能力

### 11.1 Resources

| URI | 说明 |
|---|---|
| `3gpp://catalog` | 全部已知规范（编号 + 标题 + 系列 + 版本） |
| `3gpp://catalog/{series}` | 指定系列下规范列表 |
| `3gpp://specs/{spec_id}` | 规范概览：标题、版本号、一级章节目录 |
| `3gpp://specs/{spec_id}/{release}` | 规范全文 |
| `3gpp://specs/{spec_id}/{release}/{section}` | 指定章节内容 |

### 11.2 Tools

| 工具 | 参数 | 说明 |
|---|---|---|
| `list_specs` | `series?`, `keyword?` | 浏览目录，搜索标题，不触发内容下载 |
| `search_spec` | `spec_id`, `query`, `release?` | 在指定规范内搜索，透明触发下载 |

## 十二、MCP 传输

- **stdio**：默认模式，stdin/stdout JSON-RPC，适配 Claude Desktop / opencode
- **SSE**：HTTP Server-Sent Events，适配远程 Agent
- **both**：SSE 在 goroutine，stdio 在主 goroutine，共享 Core + SQLite 连接池

## 十三、数据存储目录结构

```
data/
├── specs.db                              # SQLite 数据库（元数据 + 章节内容）
├── index/
│   ├── 38.331/
│   │   └── Rel-18/                       # per-spec per-release bleve 索引
│   └── 23.501/
│       └── Rel-18/
└── cache/
    └── 38.331/38331-g70.zip              # 可选的 ZIP 文件缓存
```

## 十四、章节导航

- `-s 5.3` → 返回 §5.3 标题 + 内容（含引导段落）+ **直接子节**的编号与标题列表
- `-s 5.3.7` → 返回 §5.3.7 标题 + 全文内容
- 非叶子节：返回内容 + 子节列表（无子节内容）
- 叶子节：返回完整内容

单个 `Content` 字段统一存储，不区分引导文本与正文。

## 十五、错误恢复

- **DB 损坏**：启动时 `PRAGMA integrity_check` 失败 → 删除重建 + 重新 sync 目录
- **流水线失败**：删除临时目录 + 清除部分 DB 条目 → 返回错误 → 下次请求自动重试
- **`repair` 命令**：手动触发 DB 校验 + 索引重建

## 十六、已知限制（v1 有意不实现）

- 无离线模式：dynareport 不可达时服务无法启动
- 无跨进程下载去重
- 无跨规范内容搜索
- 无 Procedure/IE 类型提取
- 无版本对比
- 无内容分页/截断
- store 层无接口抽象（原型阶段）
- `--release` 仅支持最新版本，不支持精确版本号选择
