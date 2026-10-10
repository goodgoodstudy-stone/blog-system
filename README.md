# 拾光博客

一个可本地复现的前后端分离博客系统，用小项目演示完整的软件工程过程。支持多博主写作、标签、注册、收藏、评论、图片上传与文章回收站。

## 一条命令启动

需要已启动的 Docker Engine 和支持 `--wait` 的 Docker Compose v2，且本机 8080 端口可用。在项目根目录运行：

```bash
docker compose up --build --wait
```

打开 [http://localhost:8080](http://localhost:8080)。首次启动会建立数据库结构并填入示例内容；再次执行不会重复生成示例文章。仅 Web 服务绑定本机地址，MySQL 和 API 不向宿主机开放端口。

默认配置无需创建 `.env`。如需修改端口或演示密码，可复制 `.env.example` 为 `.env`。`PUBLIC_ORIGIN` 填浏览器实际访问的来源（协议、域名和端口，不含路径）；需要同时从多个地址访问时，把其余来源用逗号写入 `ADDITIONAL_ORIGINS`。已经生成的演示账号密码不会因修改配置而自动重置。

### 可观测性（按需启动）

完整的接入方案、配置说明、验收与排查方法见 [可观测性接入方案](docs/OBSERVABILITY.md)。

先在 `.env` 中设置 `OTLP_ENDPOINT=alloy:4318`，然后运行：

```bash
docker compose --profile observability up --build --wait -d
```

Grafana 在 [http://localhost:3000](http://localhost:3000)，本地演示账号为 `admin`，密码由 `GRAFANA_ADMIN_PASSWORD` 配置（默认 `local-observability-only`）。首页「博客系统 · 运行概览」同时展示 API 请求、错误、延迟、CPU、内存、协程、数据库连接池，以及 MySQL 可用性、连接使用率、查询速率、慢查询和 InnoDB 缓冲池。「博客系统」文件夹另有「博客 API · Go 运行时」与 [「MySQL · 数据库健康」](http://localhost:3000/d/blog-mysql-health) 详情看板；首页顶部可以直接跳转。三张看板随 Compose 自动加载。空闲时 QPS、延迟和最近日志可能暂无数据，访问博客后会出现新样本。

MySQL 独立看板读取数据库自身状态，覆盖连接上限与错误、执行线程、慢查询及其阈值、行锁/表锁等待、InnoDB 缓冲池和磁盘读写、待完成 I/O、临时表、扫描/排序、库表容量与 MySQL stdout 警告/错误日志。`info_schema.tables` 只采集 `blog` 库。面板附有解释与排查提示；锁等待等异常计数为零是正常状态。CPU、mysqld 全部 RSS 和磁盘剩余空间需要主机/容器采集，当前尚未接入；慢查询文件日志、复制延迟、死锁详情和 SQL Digest 也未采集。

Alloy 抓取 API 内网 `:9090/metrics` 并 remote write 到 Prometheus；业务端口和 Web 代理均不暴露该路径。Alloy 内置的 MySQL exporter 每 15 秒读取 MySQL 全局状态并写入 Prometheus，使用与 API 相同的 `MYSQL_PASSWORD`，无需新镜像或手动初始化监控账号。API 另暴露 `database/sql` 连接池指标。MySQL 查询速率含监控采集自身的查询；这些指标描述数据库服务与连接池，不能代替逐条 SQL 的性能分析。Grafana 已配置 Prometheus、Tempo、Loki 数据源，支持从延迟指标的 exemplar 或日志 `traceId` 跳到 Tempo，也支持从 Tempo Span 的「Related logs」跳到 Loki 查询同一 `traceId` 的日志。HTTP 请求日志写入 stdout，包含 `requestId`、`traceId` 和路由模板；健康检查只计指标，不写请求日志。

Tempo 中的文章列表、文章详情、标签和评论读取包含数据库子 Span，可区分请求处理和数据访问耗时。文章列表的关联查询 Span 带有 `article.id` 属性。Span 存在 Tempo；详细日志存在 Loki，点击「Related logs」可查看。

Grafana 的 Traces Drilldown 使用 TraceQL 的 `rate()` 查询；Tempo 配置已启用 metrics generator 的 `local-blocks` 处理器，因此可以按服务展示请求速率。按 traceId 查 Loki 日志时，`json` 后必须再加一个管道，例如 `{project="blog-engineering-demo"} | json | traceId="你的 trace ID"`；少了第二个 `|`，`traceId=...` 只会成为 JSON 解析参数，不会过滤日志。

本地和发布 Compose 默认设置 `OBS_DEBUG_RAW=true`：记录 handler 已读取的文本请求体、写出的文本响应体，较长内容分多行记录；API 的 MySQL 包装器记录发送给驱动的 SQL 模板、参数、已扫描结果或受影响行数，请求内日志带 `traceId`。`database/sql` 使用参数化执行，SQL 模板与参数分开发送，因此日志不会伪造一条拼接后的“实际 SQL”。图片等二进制内容不打印。调试日志可能含密码、会话相关值和文章正文，留存在 Loki 数据卷，并可能随备份或日志导出保存；发布时须限制日志访问并设置适当留存时间。设置 `OBS_DEBUG_RAW=false` 可关闭，具体捕获边界见接入方案。

本地默认全量采样，`compose.release.yaml` 默认采样率为 10%，可用 `TRACE_SAMPLE_RATIO` 调整。未设置 `OTLP_ENDPOINT` 时不导出追踪，适合普通 `docker compose up`。发布环境启动可观测 profile 前应设置独立的 Grafana 管理员密码。API 通过 OTLP HTTP 将 Trace 发到 Alloy，由 Alloy 批量转发到 Tempo。Grafana Alloy 从 Docker socket 读取 API 和 MySQL 容器 stdout 日志并发送到 Loki；该 socket 即使以只读方式挂载也具有敏感权限，只应在受信任的 Docker 主机上启用。

| 演示身份 | 邮箱 | 密码 |
| --- | --- | --- |
| 管理员 | `admin@blog.local` | `Demo12345!` |
| 博主一 | `author1@blog.local` | `Demo12345!` |
| 博主二 | `author2@blog.local` | `Demo12345!` |
| 读者一 | `reader1@blog.local` | `Demo12345!` |
| 读者二 | `reader2@blog.local` | `Demo12345!` |

表中两个读者账号需要先导入扩展数据。需要更丰富的本地演示内容时，先启动服务，再手动执行 `docker compose run --rm --no-deps api demo-data`。这会补充 22 篇已发布文章、2 篇非公开文章、8 个标签、2 个读者账号及评论和收藏；重复运行不会重复插入同一批内容。普通启动和发布 Compose 不会自动添加这批扩展数据。运行 `python3 scripts/demo-traffic.py` 可发送约 20 秒的只读请求，给 Grafana 的 QPS、延迟、日志和 Trace 面板生成样本；可用 `--requests`、`--interval` 调整数量与间隔。

演示密码只用于本机体验。此 Compose 配置不用于公网部署。

### 体验路径

1. 访客浏览文章、搜索“工程”，或按标签筛选。
2. 注册普通账号，收藏文章并发表评论。
3. 以管理员身份登录，在“用户权限”中将普通账号设为博主。
4. 以博主身份新建文章、上传图片、保存草稿、预览并发布。
5. 下线、再次发布、删除文章，再从回收站恢复。

## 停止与重置

```bash
docker compose down
```

上面的命令保留文章和图片。若要清空**本项目的全部本地数据**并重新体验示例环境，运行：

```bash
docker compose down -v
docker compose up --build --wait
```

Compose 项目名固定为 `blog-engineering-demo`，以免与同名目录的其他 Compose 数据卷混用。

## 结构

```text
frontend/      React + TypeScript + Vite 页面
backend/       Go HTTP API、业务分层、MySQL 迁移与示例数据
deploy/nginx/  静态页面与 /api 代理配置
docs/          产品需求与技术方案
compose.yaml   本地一条命令启动
```

- [PRD](docs/PRD.md)：角色、业务规则和验收场景。
- [技术方案](docs/TECHNICAL_DESIGN.md)：架构、数据、API、安全与 Compose 设计。
- [GitHub Actions](docs/GITHUB_ACTIONS.md)：CI 与 GHCR 版本镜像发布。

## 测试与 TDD

实现遵循“先写可验证的行为，再实现，再重构”：文章状态与权限先写领域测试，真实 HTTP 与 MySQL 流程先写集成测试，再修复暴露的问题。新功能也按这条顺序增加用例。

```bash
cd backend && go test ./...
cd ../frontend && npm ci && npm run lint && npm run build
```

启动 Compose 后，执行真实数据库与 HTTP 集成测试：

```bash
cd backend && BLOG_BASE_URL=http://localhost:8080 go test ./integration -v
```

集成测试会创建测试账号和文章，因此完成后可用上述重置命令恢复干净的演示数据。

## 实现说明

- 浏览器访问 Web，Web 将 `/api/v1` 请求转发给 Go API；前端和后端分别构建。
- Go 后端按 HTTP、应用用例、领域规则、MySQL/文件存储分层。
- MySQL 保存用户、文章、会话、标签、收藏、评论和图片元信息；图片文件存在 Compose 数据卷中。
- 中文搜索使用 MySQL `ngram` 全文索引；单字查询使用受限的模糊匹配。
- 文章删除进入回收站；评论删除不可恢复。草稿、下线及回收站文章的图片只能由作者或管理员访问。

## 使用边界

项目面向本地演示，不包含正式对外运营所需的备份值守、邮件找回、多实例扩容及生产环境配置。请勿把演示账号和默认数据库密码直接用于公网。

## 许可证

代码与项目文档按 [MIT License](LICENSE) 开源。
