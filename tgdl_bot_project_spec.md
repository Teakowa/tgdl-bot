# TGDL Bot 项目规格说明（for AI Agent）

## 1. 项目概述

构建一个基于 Go 的 Telegram 下载机器人系统。

系统职责如下：

* 接收 Telegram 用户发送的消息
* 从消息中提取 Telegram 消息 URL
* 将下载任务写入 Cloudflare Queues
* 由独立的 Go 下载进程从 Cloudflare Queues 拉取任务
* 在本地调用 `tdl` CLI 下载目标资源
* 将任务状态持久化
* 在下载完成或失败后，通过 Telegram Bot 向用户回传结果

本项目 **不使用 Cloudflare Workers**。
Cloudflare 在该系统中 **仅作为托管队列服务** 使用。

---

## 2. 项目目标

实现一个最小可用但具备可扩展性的下载机器人，满足以下要求：

1. 使用 **Go** 作为唯一主要开发语言
2. 使用 **Telegram Bot API** 作为用户交互入口
3. 使用 **Cloudflare Queues HTTP API** 作为任务队列
4. 使用本地安装的 **`tdl` CLI** 执行真实下载
5. 支持多任务排队与并发消费
6. 支持失败重试与死信处理
7. 支持任务状态查询
8. 支持基础的幂等与重复任务保护
9. 方便后续扩展为：

   * 文件分类
   * 多目录策略
   * NAS 上传
   * Web 管理面板
   * 管理员白名单
   * 限流与配额

---

## 3. 非目标

以下内容不属于第一阶段实现范围：

1. 不实现 Web 前端
2. 不实现数据库分布式部署
3. 不实现对象存储上传
4. 不实现下载后自动再次上传文件到 Telegram
5. 不实现复杂权限系统
6. 不实现多租户
7. 不实现跨机器任务分片协调
8. 不实现 Cloudflare Worker / Durable Objects / D1 强绑定架构

---

## 4. 总体架构

```text
Telegram User
  -> Telegram Bot API
  -> Go Bot Service
       -> parse URL
       -> validate request
       -> enqueue to Cloudflare Queue
       -> reply queued message

Cloudflare Queue
  -> Go Downloader Service
       -> pull messages
       -> execute tdl CLI
       -> persist task status
       -> ack or retry
       -> notify user via Telegram
```

系统包含两个主要进程：

### 4.1 Bot Service

负责：

* 接收 Telegram 更新（Webhook 或 Long Polling，默认优先 Long Polling）
* 解析消息
* 提取合法 Telegram URL
* 生成任务 ID
* 写入 Cloudflare Queue
* 向用户返回“已入队”消息
* 响应用户状态查询命令

### 4.2 Downloader Service

负责：

* 从 Cloudflare Queue 拉取任务
* 控制下载 worker 并发数
* 调用 `tdl` CLI 下载
* 记录任务状态
* 根据结果执行 ack / retry
* 向用户发送完成或失败通知

---

## 5. 技术约束

### 5.1 技术栈

* Language: Go 1.23+
* Telegram API: Bot API
* Queue: Cloudflare Queues HTTP API + Pull Consumer
* Local downloader: `tdl` CLI
* Local storage: SQLite
* Logging: structured logs (JSON or logfmt)
* Config: environment variables + optional `.env`

### 5.2 部署约束

* Bot Service 和 Downloader Service 可部署在同一台机器，也可拆分部署
* 机器必须已安装并可调用 `tdl`
* 机器必须存在可写下载目录
* 机器必须具备稳定网络访问 Telegram / Cloudflare API

### 5.3 安全约束

* 禁止使用 `shell=True` 或拼接 shell 命令
* 必须使用 `exec.CommandContext` 调用 `tdl`
* 只接受 Telegram URL，拒绝任意命令输入
* Bot 默认仅允许白名单用户使用
* API Token、Bot Token、下载路径等必须来自环境变量

---

## 6. 目录结构

建议目录结构如下：

```text
repo-root/
  cmd/
    bot/
      main.go
    downloader/
      main.go

  internal/
    bot/
      handler.go
      parser.go
      commands.go
    telegram/
      client.go
      models.go
    queue/
      cfqueue.go
      models.go
    downloader/
      worker.go
      runner.go
      result.go
    storage/
      sqlite.go
      migrations.go
      models.go
    service/
      task_service.go
    config/
      config.go
    logging/
      logger.go
    util/
      url.go
      retry.go
      time.go

  migrations/
    001_init.sql

  scripts/
    run-bot.sh
    run-downloader.sh

  deploy/
    docker-compose.yml

  docs/
    SPEC.md
    ENV.md
    API.md

  go.mod
  go.sum
  README.md
```

---

## 6.5 登录与会话管理设计

`tdl` 的下载能力依赖一个已经登录完成的 Telegram 用户会话。
该会话与 Telegram Bot Token 无关，属于运行 `tdl` 的本地执行环境。

### 6.5.1 设计原则

* Bot 不负责完成 `tdl` 交互式登录
* Cloudflare Queue 不参与会话管理
* `tdl` 登录由部署阶段手动初始化
* Downloader Service 仅复用并检查现有会话
* 若会话不可用，Downloader Service 不得开始消费队列

### 6.5.2 第一阶段登录策略

第一阶段采用 **运维手动登录** 策略：

在 Downloader 所在机器上执行一次 `tdl login` 完成初始化，例如：

```text
tdl login -T qr -n default
```

或：

```text
tdl login -T code -n default
```

登录成功后，后续所有下载任务均复用该 namespace 对应的 session。

### 6.5.3 会话生命周期要求

系统必须显式建模以下会话状态：

* `unknown`
* `ready`
* `invalid`
* `expired`
* `blocked`

第一阶段无需将其持久化为独立表，但至少要在运行时识别：

* 当前会话是否可用
* 当前会话是否缺失
* 当前会话是否失效

### 6.5.4 启动前会话检查

Downloader Service 启动时必须执行以下顺序：

1. 加载配置
2. 检查 `tdl` 可执行文件是否存在
3. 检查下载目录是否可写
4. 检查已配置 namespace 的 Telegram session 是否可用
5. 只有检查通过后，才允许开始 pull queue

若会话不可用：

* 服务不得消费 Cloudflare Queue
* 服务应输出明确错误日志
* 服务可直接退出，或进入 degraded 状态等待人工修复

### 6.5.5 会话失效处理

若 Downloader 在运行中检测到以下错误：

* 会话失效
* 未登录
* 账号被限制
* 需要重新认证

则必须：

* 暂停继续消费新任务
* 将当前任务标记为失败或根据策略重试
* 记录明确日志
* 向管理员输出告警信息（第一阶段至少日志告警）

### 6.5.6 AI Agent 实现要求

AI agent 在实现 Downloader Service 时，必须补充一个“session preflight check”步骤。
该步骤必须发生在 pull consumer 启动之前。

第一阶段不实现：

* 通过 Bot 交互式完成手机号登录
* 通过 Bot 输入验证码
* 通过 Bot 输入二步验证密码
* 自动刷新登录态

这些能力仅可在后续阶段作为管理员能力扩展。

---

## 7. 配置项

所有配置通过环境变量读取。

### 7.1 Telegram

* `TELEGRAM_BOT_TOKEN`
* `TELEGRAM_API_BASE`（可选，默认官方）
* `TELEGRAM_USE_WEBHOOK`（默认 false）
* `TELEGRAM_WEBHOOK_URL`（可选）
* `TELEGRAM_ALLOWED_USER_IDS`（逗号分隔）

### 7.2 Cloudflare Queue

* `CF_ACCOUNT_ID`
* `CF_QUEUE_ID`
* `CF_API_TOKEN`
* `CF_QUEUE_BATCH_SIZE`（默认 5）
* `CF_QUEUE_VISIBILITY_TIMEOUT_MS`（默认 900000，即 15 分钟）
* `CF_QUEUE_PULL_INTERVAL_MS`（默认 3000）

### 7.3 Downloader

* `TDL_BIN`（默认 `tdl`）
* `TDL_NAMESPACE`（默认 `default`）
* `TDL_STORAGE`（可选，`tdl` storage 配置）
* `TDL_LOGIN_REQUIRED`（默认 true）
* `TDL_LOGIN_CHECK_ON_START`（默认 true）
* `DOWNLOAD_DIR`
* `TDL_GROUP`（默认 true）
* `TDL_SKIP_SAME`（默认 true）
* `TDL_TASK_CONCURRENCY`（传给 `tdl -l`，默认 1）
* `TDL_THREAD_CONCURRENCY`（传给 `tdl -t`，默认 4）
* `DOWNLOADER_WORKERS`（默认 2）
* `TASK_TIMEOUT_MINUTES`（默认 60）

### 7.4 Storage

* `SQLITE_PATH`（默认 `./data/tasks.db`）

### 7.5 Runtime

* `LOG_LEVEL`
* `ENV`

---

## 8. 核心数据模型

### 8.1 Queue Message

```json
{
  "task_id": "uuid",
  "chat_id": 123456789,
  "user_id": 123456789,
  "url": "https://t.me/c/xxx/123",
  "options": {
    "group": true,
    "skip_same": true
  },
  "created_at": "2026-03-21T00:00:00Z"
}
```

### 8.2 Task Entity

字段建议：

* `task_id` string, PK
* `chat_id` int64
* `user_id` int64
* `url` string
* `status` string
* `created_at` datetime
* `updated_at` datetime
* `started_at` nullable datetime
* `finished_at` nullable datetime
* `retry_count` int
* `lease_id` nullable string
* `download_dir` nullable string
* `output_summary` nullable text
* `error_message` nullable text
* `exit_code` nullable int
* `idempotency_key` string

### 8.3 Status Enum

* `queued`
* `running`
* `done`
* `failed`
* `retrying`
* `dead_lettered`

---

## 9. Bot 交互行为

### 9.1 支持的输入

机器人需要支持以下输入：

1. 直接发送 Telegram 消息 URL
2. 命令：`/start`
3. 命令：`/help`
4. 命令：`/status <task_id>`
5. 命令：`/last`

### 9.2 URL 识别规则

仅接受以下域名：

* `t.me`
* `telegram.me`

至少支持以下常见格式：

* `https://t.me/<channel>/<message_id>`
* `https://t.me/c/<chat_id>/<message_id>`
* `https://telegram.me/<channel>/<message_id>`

可接受消息中包含额外文本，但需提取第一个合法 URL。

### 9.3 用户白名单

若启用白名单，则非白名单用户发送任何消息时：

* 不创建任务
* 返回固定拒绝消息

---

## 10. 任务流

### 10.1 入队流程

当 Bot 收到合法 URL：

1. 检查用户是否在白名单中
2. 解析 URL
3. 生成 `task_id`
4. 生成 `idempotency_key`
5. 将任务先写入 SQLite，状态为 `queued`
6. 调用 Cloudflare Queue HTTP API 入队
7. 成功则回复用户：

   * 已入队
   * task_id
8. 若入队失败：

   * 标记数据库状态为 `failed`
   * 回复用户错误信息

### 10.2 下载流程

Downloader Service 周期性执行：

1. 从 Cloudflare Queue pull 一批消息
2. 对每条消息解析任务体
3. 检查任务状态是否已完成
4. 若已完成则直接 ack
5. 若未完成则将任务置为 `running`
6. 调用 `tdl` 执行下载
7. 根据结果更新数据库
8. 成功则 ack
9. 失败则根据错误类型决定 ack 或 retry

---

## 11. `tdl` 调用规范

必须通过 `exec.CommandContext` 调用，不可通过 shell 字符串。

默认命令格式：

```text
tdl dl -u <URL> -d <DOWNLOAD_DIR> --group --skip-same -l 1 -t 4
```

实现时需允许通过配置调整：

* group
* skip-same
* `-l`
* `-t`

### 11.0 Session 复用要求

所有 `tdl` 命令必须与预先初始化完成的 session namespace 保持一致。

调用 `tdl` 时必须能够指定或继承以下配置：

* namespace
* storage

Downloader Service 必须保证：

* 所有下载任务使用同一组明确配置的 session 参数
* 不允许隐式混用多个未知 session
* 不允许在任务执行过程中临时触发交互式登录

### 11.1 进程执行要求

* 捕获 stdout 和 stderr
* 记录退出码
* 支持 context timeout
* 超时后强制终止进程并标记失败

### 11.2 输出处理

下载结果需要提取至少以下信息：

* 是否成功
* 下载目录
* 标准输出摘要
* 标准错误摘要
* 耗时

不要求第一阶段解析所有下载文件列表，但至少保存原始输出摘要。

---

## 12. 幂等与重复任务策略

系统必须按“至少一次投递”设计。

### 12.1 幂等要求

* 同一任务可能因 Cloudflare Queue retry 被重复投递
* 消费端不得假设消息只会出现一次

### 12.2 幂等实现建议

定义：

* `idempotency_key = sha256(user_id + "|" + normalized_url)`

规则：

* 若数据库中已有 `done` 状态的同 `idempotency_key` 任务，可直接 ack，不再重复下载
* 若已有 `running` 状态的相同 key，可直接 retry 或 ack，避免并发重复下载
* 若已有 `failed` 但可重试，则允许新任务继续执行

---

## 13. 错误分类

必须区分“可重试错误”和“不可重试错误”。

### 13.1 可重试错误

* Cloudflare API 短暂失败
* Telegram API 短暂失败
* 网络超时
* 本地 IO 暂时失败
* `tdl` 因网络问题返回失败

处理：

* 记录错误
* 更新任务状态为 `retrying`
* 对消息执行 retry

### 13.2 不可重试错误

* 非法 URL
* 用户无权限访问该链接目标
* `tdl` 未安装
* 下载目录不可写
* 配置缺失
* 白名单拒绝
* `tdl` 会话未初始化
* `tdl` 登录态失效且需要人工重新登录

处理：

* 更新任务状态为 `failed`
* ack 消息，不再重试
* 通知用户失败原因

---

## 14. Telegram 通知规则

### 14.1 入队成功

消息模板：

```text
任务已加入队列
Task ID: <task_id>
URL: <url>
```

### 14.2 下载成功

消息模板：

```text
下载完成
Task ID: <task_id>
保存目录: <download_dir>
耗时: <duration>
```

### 14.3 下载失败

消息模板：

```text
下载失败
Task ID: <task_id>
原因: <error_summary>
```

### 14.4 状态查询

`/status <task_id>` 返回：

* 当前状态
* URL
* 创建时间
* 完成时间
* 最近错误

### 14.5 最近任务

`/last` 返回当前用户最近 10 条任务的摘要。

---

## 15. SQLite 设计

最少需要一个 `tasks` 表。

建议建表字段：

```sql
CREATE TABLE tasks (
  task_id TEXT PRIMARY KEY,
  chat_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  url TEXT NOT NULL,
  status TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  retry_count INTEGER NOT NULL DEFAULT 0,
  lease_id TEXT,
  download_dir TEXT,
  output_summary TEXT,
  error_message TEXT,
  exit_code INTEGER,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  started_at DATETIME,
  finished_at DATETIME
);

CREATE INDEX idx_tasks_user_created_at ON tasks(user_id, created_at DESC);
CREATE INDEX idx_tasks_idempotency_key ON tasks(idempotency_key);
CREATE INDEX idx_tasks_status ON tasks(status);
```

---

## 16. Cloudflare Queue 客户端要求

实现一个独立 `queue` 包，封装以下能力：

### 16.1 Producer

* `Enqueue(ctx, message) error`
* `EnqueueBatch(ctx, messages) error`

### 16.2 Consumer

* `Pull(ctx, batchSize, visibilityTimeoutMs) ([]Message, error)`
* `Ack(ctx, leaseIDs []string) error`
* `Retry(ctx, leaseIDs []string) error`
* `AckAndRetry(ctx, ackLeaseIDs, retryLeaseIDs []string) error`

### 16.3 设计要求

* 统一 HTTP client
* 支持 timeout
* 支持 structured logs
* 对 Cloudflare 错误返回原始响应摘要

---

## 17. Downloader Worker 设计

### 17.1 Worker Pool

Downloader Service 必须实现固定 worker 池：

* 启动 N 个 goroutine worker
* puller 负责拉取消息
* worker 负责处理任务

### 17.2 推荐执行模型

1. puller 定时从队列拉消息
2. 每条消息投递到本地 channel
3. worker 从 channel 获取任务
4. 执行下载
5. 将 ack/retry 决策返回给 coordinator
6. coordinator 汇总后调用 Cloudflare ack API

### 17.2.1 会话预检

在 worker pool 启动前，Downloader Service 必须执行 session preflight check。

该检查至少应覆盖：

* `tdl` 二进制存在
* namespace 配置有效
* storage 配置可访问
* 当前登录态可用于执行下载

若预检失败：

* 不创建 pull loop
* 不启动 worker pool
* 返回明确错误

### 17.3 关闭行为

当服务收到 SIGTERM / SIGINT：

* 停止新的 pull
* 等待当前 worker 完成或超时
* 未完成任务不要 ack
* 优雅退出

---

## 18. 日志要求

日志必须可机器解析，建议 JSON。

每条日志尽量包含：

* timestamp
* level
* component
* task_id
* user_id
* url
* message
* error

关键日志点：

* bot 收到更新
* URL 提取成功/失败
* 任务写库
* 任务入队成功/失败
* pull 成功/失败
* worker 开始任务
* `tdl` 开始/结束
* ack/retry 成功/失败
* Telegram 通知成功/失败

---

## 19. 测试要求

### 19.1 单元测试

至少覆盖：

* URL 解析
* 白名单判断
* idempotency key 生成
* Cloudflare API request body 生成
* `tdl` runner 参数拼装
* 错误分类逻辑

### 19.2 集成测试

至少覆盖：

* Bot 收到 URL 后写库并入队
* Downloader 拉取任务并调用假 runner
* 任务成功后状态更新为 done
* 任务失败后状态更新并 retry

### 19.3 Mock 策略

* Telegram API 使用 mock server
* Cloudflare Queue API 使用 mock HTTP server
* `tdl` runner 抽象成 interface，可注入 fake runner

---

## 20. 验收标准

第一阶段完成后，必须满足以下验收项：

1. 用户向机器人发送合法 `t.me` 链接，机器人能回复“已加入队列”
2. 任务能够成功写入 Cloudflare Queue
3. Downloader 能够从 Cloudflare Queue 拉取任务
4. Downloader 能够调用 `tdl` CLI 下载
5. 下载成功后任务状态变为 `done`
6. 下载失败时能区分 retryable / non-retryable
7. retryable 失败能触发重试
8. 用户可通过 `/status <task_id>` 查询状态
9. 用户可通过 `/last` 查看最近任务
10. 非白名单用户无法使用
11. 服务重启后 SQLite 中的历史任务仍可查询
12. 同一 URL 重复投递时不会造成明显重复下载

---

## 21. 实现优先级

### Phase 1

* Bot Long Polling
* URL 解析
* SQLite 持久化
* Cloudflare Queue 入队
* Pull consumer
* `tdl` 执行
* `tdl` 手动初始化登录
* Downloader 启动前 session check
* 状态通知
* `/status`
* `/last`

### Phase 2

* Webhook 模式
* 消息完整转发执行链路（URL -> queue -> tdl forward）
* 转发结果解析与状态回传增强
* 管理员命令
* 按用户限流
* DLQ 可视化处理（面向转发失败重试）

### Phase 3

* NAS 上传
* Web 管理面板
* 多实例下载器
* 指标监控

---

## 22. AI Agent 执行要求

面向 AI agent 的实现要求：

1. 优先生成可运行的最小实现，不要先做过度抽象
2. 所有外部依赖必须通过 interface 隔离：

   * Telegram Client
   * Queue Client
   * Storage
   * TDL Runner
3. 不要引入重量级框架
4. 先写 migrations 和数据模型
5. 先完成 happy path，再补错误处理
6. 每完成一个模块，补对应单元测试
7. 保持代码风格一致，避免过度泛化
8. README 中必须包含本地运行步骤

---

## 23. 建议的首批实现任务拆解

1. 初始化 Go module
2. 实现 config loader
3. 实现 SQLite storage 和 migrations
4. 定义 task model 和 repository
5. 实现 Telegram client
6. 实现 Bot long polling handler
7. 实现 URL parser
8. 实现 Cloudflare queue producer
9. 实现 Cloudflare queue pull consumer
10. 实现 `tdl` runner
11. 实现 downloader worker pool
12. 实现状态查询命令
13. 增加测试
14. 编写 README

---

## 24. 最终交付物

AI agent 最终应输出：

1. 完整 Go 项目代码
2. 可运行的 `cmd/bot` 和 `cmd/downloader`
3. SQLite migration 文件
4. README
5. `.env.example`
6. 基础测试
7. Docker 或 docker-compose 示例（可选但推荐）

---

## 25. 补充说明

该项目的真实下载能力取决于本地 `tdl` 登录的 Telegram 账号权限。
Bot 只是入口层，不等于下载权限本身。

因此实现中必须默认假设：

* 某些 URL 即使格式合法，也可能因为权限不足下载失败
* 这类失败要正常反馈给用户，而不是视为系统错误
* 若 downloader 所在机器未执行过 `tdl login`，则系统不应开始消费队列
* 登录态是部署前置条件，而不是运行时由普通用户触发的能力

### 25.1 部署前置步骤

在第一阶段部署文档中，必须明确写入以下前置步骤：

1. 安装 `tdl`
2. 在目标机器上执行一次 `tdl login`
3. 确认 namespace 与 storage 配置
4. 验证 downloader 进程能够复用该 session
5. 完成后才允许启动正式消费进程

### 25.2 后续可扩展方向

后续如需增强登录管理，可考虑新增管理员能力：

* 查询当前 `tdl` 登录状态
* 暂停下载消费
* 恢复下载消费
* 登录状态异常时发送管理员告警

但第一阶段不实现通过 Bot 进行交互式登录。
