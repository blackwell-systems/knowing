[English](../../README.md) · **简体中文** · [Русский](README.ru.md) · [हिन्दी](README.hi.md) · [العربية](README.ar.md)

<p align="center">
  <img src="../../assets/knowing-banner.png" alt="knowing" width="600">
</p>

<p align="center">
  <a href="https://github.com/blackwell-systems"><img src="https://raw.githubusercontent.com/blackwell-systems/blackwell-docs-theme/main/badge-trademark.svg" alt="Blackwell Systems"></a>
  <a href="https://zenodo.org/records/20342255"><img src="https://zenodo.org/badge/DOI/10.5281/zenodo.20342255.svg" alt="DOI"></a>
  <a href="#mcp-tools"><img src="https://img.shields.io/badge/MCP_tools-28%20tools%20%2B%208%20resources-brightgreen.svg" alt="MCP Tools"></a>
  <a href="#languages-and-formats"><img src="https://img.shields.io/badge/languages_and_formats-26-blue.svg" alt="Languages and Formats"></a>
  <a href="../../LICENSE"><img src="https://img.shields.io/badge/license-Apache_2.0-blue.svg" alt="License"></a>
</p>

---

自适应的代码智能引擎。观测自身的图密度并自动调整检索策略。38 种边类型、28 个 MCP 工具、263 个等价类、密码学证明。随规模扩大而变得更聪明，而非更迟钝。

---

> [!NOTE]
> **基于已发表的研究成果：** [Content-Addressing as a Computation Primitive for Software Relationship Intelligence](https://zenodo.org/records/20342255)（DOI: 10.5281/zenodo.20342255）

你的架构图说服务 A 调用服务 B。你能证明吗？

**knowing 能。** 它为提取出的代码关系构建一个内容寻址的图，将其快照为绑定到某个 git commit 的 Merkle 树，并生成可离线验证的密码学证明。Agent 用它获取排序后的上下文。安全团队用它做审计。平台团队用它将代码与生产环境的 trace 进行比对。

它随着你每次使用而变得更好。当代码变更时，陈旧的知识会自动过期。

```bash
brew install blackwell-systems/tap/knowing
```

```json
{ "mcpServers": { "knowing": { "command": "knowing", "args": ["mcp", "--watch"] } } }
```

就这么简单。MCP 服务器在首次启动时会自动索引你的仓库。无需下载模型，无需 API key。你的 agent 现在就拥有了排序后的上下文、影响范围（blast radius）、测试范围，以及在活动会话期间不断改进结果的隐式噪声降权。

**验证它是否生效：** 询问你的 agent：*"使用 context_for_task 工具查找与 [你知道存在于代码中的某个东西] 相关的符号。"* 你应当看到带有分数和文件路径、来自你代码库的排序符号。如果结果为空，说明仓库仍在索引中（首次启动需 10-30 秒）。如果结果看起来不相关，请参阅 [故障排查](../../docs/guide/cli.md#troubleshooting)。

> **没有使用 AI agent？** 直接跳到下方的 [CLI 用法](#path-b-cli-usage-explore-the-graph-yourself)。

| 你想要... | 从这里开始 |
|---|---|
| 为你的 AI agent 提供图排序上下文 | [MCP 配置](#mcp-integration) |
| 从 CLI 探索图 | [CLI 用法](#path-b-cli-usage-explore-the-graph-yourself) |
| 理解检索的工作原理 | [简介](../../docs/guide/introduction.md) |
| 用密码学证明进行审计 | [审计与合规](../../docs/guide/audit-compliance.md) |

---

## 三样东西，一套架构

knowing 是构建在同一个基础（带有分层 Merkle 树的内容寻址图）之上的三个产品：

**1. 面向 AI agent 的上下文引擎**
一次调用即返回某个任务最相关的符号，按图中心性、时近性和习得有用性排序，并打包以适配你的 token 预算。263 个框架等价类在关键字失效时弥合词汇差异。工具调用减少 47%。token 减少 84%。结果随反馈而改进。

**2. 面向合规的审计原语**
每个图状态都是绑定到某个 git commit 的 Merkle 根。`knowing prove` 生成一个证明某关系曾经存在的密码学证明。`knowing verify` 离线校验它。`knowing fsck` 在 98ms 内校验整个图。供应链检测提取凭据访问、进程派生和网络外泄边，用以标记结构上可疑的代码。

**3. 会学习的噪声降权**
被返回但从未被 agent 使用的符号会在后续查询中被降权。当代码变更时，反馈会自动过期（通过 package 的 Merkle 根验证）。系统在活动会话期间变得更精确。这正是 knowing 所围绕构建的特性。

**这些并非各自独立的功能。** 它们是内容寻址的结构性结果：让上下文可缓存的那个哈希，同样让它可证明；检测陈旧性的那个 Merkle 根，同样让陈旧反馈过期。

---

## 它能回答什么

**为你的 agent：**
- "我要修改这个函数。什么会被破坏？"（跨调用方、测试、路由、仓库的影响范围）
- "给我这个任务的 50,000 个 token 的上下文。"（图排序，而非 grep 搜索）
- "应该运行哪些测试？"（调用图遍历，98% 精确率）

**为你的平台团队：**
- "这条路由在生产环境中被使用吗？"（静态分析 + OTel 运行时 trace）
- "在某个特定快照时，服务图是什么样子？"（快照链，每个根都绑定到一个 git commit）

**为你的安全团队：**
- "证明服务 A 在此 commit 处调用服务 B。"（Merkle 证明，可离线验证）
- "证明这个依赖并不存在。"（通过排序叶子的缺失证明）
- "生成一份合规报告。"（`knowing audit -proofs`，一条命令）
- "这个 package 是否读取凭据并派生进程？"（`knowing audit-supply-chain --scan-all`）

---

## 数据

| 项目 | 结果 |
|---|---:|
| 跨系统检索 | **P@10=0.330 冷启动**（302 个任务、17 个仓库、8 种语言） |
| 对比竞品 | 3.79x codegraph（19K stars）、6.00x GitNexus、6.35x Gortex、22.0x grep |
| 等价类 | 277 个手工整理 + 从使用中习得，将词汇桥接到符号（+57% P@10） |
| 噪声降权 | 逐聚类隐式反馈：R@10 +5.2%、MRR +12.6%（Django 5 轮） |
| 节省的工具调用 | 减少 47%（一次上下文调用替代反复的 grep+read） |
| 节省的 token | 减少 84% token（GCF 线格式） |
| 重复查询速度 | 快 93x（Merkle 键控的子图缓存） |
| Merkle diff | 在 100K 边规模下比全边扫描快 517x |
| 测试范围 | 98% 精确率、82% 召回率 |
| 图完整性检查 | 98ms（24,936 条边） |
| 证明生成 | 生成 72us、验证 1.2us |
| 反馈过期 | 代码变更时 100% 过期、11% 开销 |
| 索引吞吐量 | 16 个仓库（8 种语言）约 60s |
| 语言覆盖 | 16/16 仓库通过（Go、Python、TS、Rust、Java、C#、Ruby、多语言） |
| 边类型 | 38（含供应链：reads_env、executes_process） |

所有基准测试均可复现。跨系统基准（P@10=0.330）使用固定到精确 commit 的 17 个仓库，并附有 [语料清单](../../bench/cross-system/corpus/MANIFEST.yaml) 和 [配置脚本](../../bench/cross-system/corpus/corpus-setup.sh) 以支持完整的从零复现。协议细节参见 [METHODOLOGY.md](../../bench/cross-system/METHODOLOGY.md)。

---

## 快速开始

### 路径 A：MCP 服务器（推荐用于 AI agent）

```bash
# 1. Install
brew install blackwell-systems/tap/knowing
# Or: npm install -g @blackwell-systems/knowing
# Or: pip install knowing
# Or: go install github.com/blackwell-systems/knowing/cmd/knowing@latest

# 2. Add to your agent config (.mcp.json, Claude Code settings, etc.)
#    See "MCP Integration" below for the config block.
#    The server auto-indexes your repo on first launch. Done.
```

### 路径 B：CLI 用法（自己探索图）

```bash
# 1. Install (same as above)
brew install blackwell-systems/tap/knowing

# 2. Index your repo
knowing add .

# 3. Verify the index worked
knowing stats
# You should see node and edge counts. A healthy TypeScript repo with 50K LOC
# typically produces 2K-10K nodes and 5K-30K edges. If you see very few edges,
# the extractors may not have found your code (check language support below).

# 4. Get context for a task
knowing context -task "refactor auth middleware" -format gcf

# 5. Check graph integrity
knowing fsck
```

### 验证你的配置

索引完成后，运行这些命令以确认一切正常：

```bash
# Show node/edge counts, repos, snapshots
knowing stats

# Search for a symbol you know exists in your code
knowing query "MyKnownFunction"

# Check graph integrity (should report 0 errors)
knowing fsck

# If results seem wrong, check if the graph is stale
knowing stale
```

如果 `knowing stats` 显示零个节点或极少的边，请参阅下方的
[故障排查](../../docs/guide/cli.md#troubleshooting)。

### 更多 CLI 命令

```bash
# Find affected tests
knowing test-scope -files internal/auth/middleware.go

# Explain why a symbol ranked where it did
knowing why -task "refactor auth" -symbol "SessionHandler"

# Prove a relationship exists (cryptographic Merkle proof)
knowing prove -source "AuthService" -target "SessionStore"

# Verify offline (no database needed)
knowing verify proof.json

# Check if the graph is stale (CI gate: exits 1 if stale)
knowing stale

# Supply chain audit (scan all files for suspicious patterns)
knowing audit-supply-chain --scan-all

# Remove a repo (evicts all data: nodes, edges, snapshots, feedback)
knowing remove ./path/to/repo
```

完整的命令参考请见 [CLI 参考](../../docs/guide/cli.md)。

### MCP 集成

将 MCP 服务器添加到你的 agent。配置在各处都相同，只有文件路径不同。

| Agent | 配置文件 |
|-------|-------------|
| **Claude Code** | `.mcp.json`（项目根目录）或 `~/.claude/mcp.json`（全局） |
| **Cursor** | `.cursor/mcp.json` |
| **Windsurf** | `~/.codeium/windsurf/mcp_config.json` |
| **VS Code**（Copilot、Continue、Cline、Roo） | `.vscode/mcp.json` |
| **Zed** | `~/.config/zed/settings.json` 中的 `"context_servers"` 下 |
| **Codex**（OpenAI） | `codex.json` 或 `--mcp-config` 标志 |
| **JetBrains** | Settings > Tools > MCP Servers |

```json
{
  "mcpServers": {
    "knowing": {
      "command": "knowing",
      "args": ["mcp", "--watch"],
      "transport": "stdio"
    }
  }
}
```

`--watch` 标志会在文件变更时重新索引。你的 agent 始终查询到最新数据。无需手动执行 `knowing index` 或指定数据库路径：MCP 服务器在首次启动时会自动索引该 git 仓库，并将其注册到花名册中供后续会话使用。

Embedding 默认关闭（在冷启动基准上确认为中性影响）。如需试验，使用 `--embeddings` 启用。图结构和等价类承载检索质量。

**你的 agent 会获得什么：** 关键工具是 `context_for_task`。当你的 agent 用一段任务描述调用它时，knowing 会返回打包进 token 预算的、经过排序的相关代码符号。这替代了 grep-read 循环。其他有用的工具：`blast_radius`（如果我改动这个，什么会被破坏？）、`test_scope`（该运行哪些测试？）、`explain_symbol`（为什么它排在这里？）。全部 28 个工具参见 [MCP 工具参考](../../docs/guide/mcp-tools.md)。

**验证它是否生效：**

1. 与你的 agent 开始一个会话
2. 询问：*"使用 context_for_task 工具查找与 [你代码中某个具体东西] 相关的符号"*
3. 你应当看到带有分数和文件路径、来自你代码库的排序符号

如果结果为空：仓库可能仍在索引中（首次启动需 10-30 秒）。如果结果看起来不相关：在任务描述中使用具体的符号名（例如 "find the `AuthMiddleware` handler" 而不是 "find auth code"）。你也可以从 CLI 验证：

```bash
knowing stats          # should show nodes and edges
knowing query "MyFunc" # should find symbols you recognize
```

对于 HTTP 传输（多 agent、守护进程模式）：

```bash
knowing serve -addr :8100 .
```

```json
{
  "mcpServers": {
    "knowing": {
      "url": "http://localhost:8100",
      "transport": "streamable-http"
    }
  }
}
```

---

## 为什么它有效

**Git 对文件做版本管理。knowing 对代码的理解做版本管理。**

整个系统建立在一个理念之上：内容寻址的身份标识。每个符号、关系和快照都经过 SHA-256 哈希。这一个选择带给你：

- **免费的陈旧性检测。** 文件变更 = 新哈希 = 无需扫描即可知晓陈旧的边。
- **免费的缓存。** 相同的 package 根 = 相同的结果。未变更查询上 93x 加速。
- **免费的完整性。** 校验所有存储的哈希和快照链的连续性。98ms。
- **免费的历史。** 每个快照都是绑定到某个 git commit 的 Merkle 根。沿链行走。
- **免费的反馈过期。** 反馈存储 package 的 Merkle 根。代码变更 = 根变更 = 旧反馈不可见。
- **免费的证明。** 从叶子到根的 Merkle 路径本身就是一个自包含的密码学证明。

| | Git | knowing |
|---|---|---|
| 它对什么做版本管理 | 文件内容 | 代码关系及其含义 |
| 存储单元 | blob | node + edge + provenance + confidence |
| 身份标识 | `sha256(content)` | `sha256("node\0" + repo + package + name + kind)` |
| 快照 | blob 的树 | 分层 Merkle：repo -> package -> edge-type -> leaf |
| Diff | 哪些行变了 | 哪些 package 变了、什么被破坏、什么是新增 |
| 历史 | 代码曾经的样子 | 代码库对自身理解的样子 |

---

## 它如何工作

```
+------------------------------------------------------------------+
|                         knowing daemon                            |
+----------------+------------------------+--------------------------+
|   Indexer      |     Graph Store        |      MCP Server          |
|                |                        |                          |
| 23 extractors  | Content-addressed      | 28 tools + 8 resources   |
| tree-sitter    | SQLite + Merkle tree   | stdio / HTTP (1.8s index)|
| LSP + SCIP     | 38 edge types          | GCF / GCB / JSON         |
| OTel traces    | Subgraph cache (93x)   | PackRoot dedup (99%)     |
|                | Embedding vector cache | Embedding re-ranker      |
|                | Community detection    | Supply chain audit       |
+----------------+------------------------+--------------------------+
```

两个平面：
- **执行：** 索引仓库、提取符号和关系、摄入 trace、存储快照。
- **智能：** 从存储的图计算影响范围、上下文包、测试范围、反馈、社区。

这条边界很重要：智能功能读取图并产出派生结果。它们无法破坏图的事实。糟糕的排序会产出糟糕的推荐；但它无法使一个证明失效。

---

## 能力

### 语言与格式

| 语言/格式 | 提取器 | 框架/模式检测 |
|---|---|---|
| Go | tree-sitter + `go/packages` + SCIP | net/http, gin, echo, chi, gorilla/mux |
| TypeScript/JavaScript | tree-sitter | Express.js, Fastify, Hono, NestJS, Next.js |
| Python | tree-sitter | Flask, FastAPI, Django |
| Rust | tree-sitter | Actix, Axum, Rocket |
| Java | tree-sitter | Spring annotations |
| C# | tree-sitter | ASP.NET attributes |
| Protocol Buffers | tree-sitter | service、message、enum、RPC 声明 |
| Terraform (HCL) | tree-sitter | resource、data、module、variable 声明 |
| SQL | tree-sitter | 表、视图、函数、存储过程、FK 边 |
| Kubernetes YAML | yaml.v3 | deployments、services、configmaps、label-selector 边 |
| CloudFormation/SAM | yaml.v3 | resources、!Ref/!GetAtt/!Sub 交叉引用 |
| Docker Compose | yaml.v3 | services、ports、networks、depends_on 链接 |
| GitHub Actions | yaml.v3 | workflows、jobs、steps、action 引用 |
| Serverless Framework | yaml.v3 | functions、events、resource 引用 |
| CSS/SCSS | tree-sitter | 选择器、自定义属性、var() 依赖 |
| Event/MQ 模式 | multi-language | Kafka、NATS、SQS、RabbitMQ 发布/订阅 |
| OpenAPI/JSON Schema | json/yaml | endpoints、models、$ref 解析 |
| Dockerfile | parser | FROM 基础镜像、COPY --from 多阶段依赖、EXPOSE 端口 |
| Makefile | parser | target 依赖、include 指令、variable 引用 |
| Helm Charts | yaml.v3 | chart 依赖、template 引用、values 注入 |
| GitLab CI | yaml.v3 | job needs、extends 模板、include 文件、artifacts |
| package.json (npm) | json | dependencies、devDependencies、peerDependencies、scripts |
| GraphQL | parser | 类型定义、字段类型引用、interface 实现 |
| Ruby | tree-sitter | 类、模块、方法定义、require 边 |
| .env 文件 | parser | 环境变量声明、跨文件引用 |

所有提取器通过多分派逐文件触发；结果被合并。tree-sitter 以 0.7 置信度产出边（`ast_inferred`）；`go/packages` 和 SCIP 以 0.95-1.0 置信度产出（`ast_resolved`、`scip_resolved`）。

### MCP 工具

| 工具 | 用途 |
|---|---|
| `index_repo`, `graph_query`, `repo_graph` | 构建和检视图 |
| `cross_repo_callers`, `blast_radius`, `trace_dataflow`, `flow_between` | 理解影响与路径 |
| `snapshot_diff`, `semantic_diff`, `pr_impact`, `stale_edges` | 比较图状态并审查变更 |
| `runtime_traffic`, `dead_routes`, `trace_stats` | 查询运行时观测到的关系 |
| `context_for_task`, `context_for_files`, `context_for_pr`, `explain_symbol` | 为 agent 提供排序上下文 |
| `ownership`, `ownership_query`, `test_scope`, `communities`, `plan_turn`, `feedback` | 分派工作、查询代码所有者/作者、选择测试、改进排序 |
| `prove`, `prove_absent`, `fsck` | 密码学证明、缺失证明、完整性校验 |
| `untrack_repo` | 驱逐某仓库的所有数据（节点、边、文件、快照、反馈、任务记忆、图注记） |

MCP prompts：`refactor_safely`、`review_pr`、`investigate_dead_code`。

### MCP 资源

8 个只读资源，无需工具调用即可为 agent 定位方向：

| 资源 | 它返回什么 |
|---|---|
| `knowing://report` | 图规模、顶部种类、热点数量、快照年龄 |
| `knowing://schema` | 节点种类、边类型、来源层级、哈希格式 |
| `knowing://stats` | 按仓库、种类和边类型的计数 |
| `knowing://repos` | 所有被追踪的仓库及其计数和最后索引时间 |
| `knowing://session` | 上下文调用、已服务的符号、缓存命中/未命中、运行时长 |
| `knowing://index-health` | 健康/陈旧/损坏状态、完整性检查 |
| `knowing://communities` | 社区列表及其内聚度和 Merkle 根 |
| `knowing://community/{id}` | 单个社区详情（资源模板） |

---

## 线格式

| 格式 | 用途 | 相较 JSON 的节省 |
|---|---|---|
| **GCF**（Graph Compact Format） | LLM 消费：面向行、位置字段 | token 减少 84% |
| **GCB**（Graph Compact Binary） | 服务传输与缓存：varint、长度前缀 | 字节减少 74% |
| **JSON** | 人工调试、通用消费者 | 基线 |

GCF 使用 `|` 分隔的字段和本地 ID（`$1 -> $3`）来替代重复的限定名。可被 LLM 解析，同时在相同的 token 预算内容纳 5x 的图上下文。会话有状态的去重使重复符号减少 47%。

---

## 当前边界

- 静态影响范围沿 `calls` 边追踪；其他边类型提供上下文，而非遍历。
- 运行时工具需要 OpenTelemetry trace 摄入；没有 trace 就没有观测。
- LSP 增强：Go、TypeScript、Python、Rust、Java、C#。从项目标记自动检测。其他语言回退到 tree-sitter。
- Embedding 默认关闭（在冷启动基准上确认为中性影响，session 23）。使用 `--embeddings` 选择开启。

---

## 文档

| 文档 | 内容 |
|---|---|
| [简介](../../docs/guide/introduction.md) | 工作原理、检索流水线详解、5 分钟上手 |
| [架构](../../docs/architecture/) | 系统设计、schema、内容寻址、守护进程模型 |
| [功能](../../docs/guide/features.md) | 实现清单、入口点、限制 |
| [审计与合规](../../docs/guide/audit-compliance.md) | Merkle 证明、fsck、快照链、CI 门禁 |
| [CLI 参考](../../docs/guide/cli.md) | 命令、标志、示例、[故障排查](../../docs/guide/cli.md#troubleshooting) |
| [MCP 工具](../../docs/guide/mcp-tools.md) | 工具 schema、参数、返回格式 |
| [边类型](../../docs/architecture/edge-types.md) | 关系语义与来源 |
| [上下文打包](../../docs/architecture/context-packing.md) | RWR、HITS、排序、token 预算 |
| [Embedding 重排器](../../docs/architecture/embedding-reranker.md) | 本地推理、向量缓存、延迟画像 |
| [运行时 Trace](../../docs/operations/runtime-traces.md) | OTel 摄入与运行时置信度 |
| [线格式](../../docs/architecture/wire-formats.md) | GCF、GCB、JSON 格式与基准 |
| [路线图](../../docs/roadmap.md) | 已完成的工作流与下一步优先事项 |
| [基准测试](../../bench/README.md) | 带性能契约的可复现价值基准 |
| [研究](../../docs/research/content-addressing-as-computation-primitive.md) | knowing 所依据的论点：内容寻址作为一种计算原语（[DOI: 10.5281/zenodo.20342255](https://zenodo.org/records/20342255)） |
| [Hooks](../../hooks/README.md) | Claude Code hook 集成 |

## 许可

Apache-2.0 (c) 2026 Dayna Blackwell / Blackwell Systems。参见 LICENSE 和 NOTICE。
