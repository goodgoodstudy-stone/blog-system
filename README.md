# 拾光博客

一个可本地复现的前后端分离博客系统，用小项目演示完整的软件工程过程。支持多博主写作、标签、注册、收藏、评论、图片上传与文章回收站。

## 一条命令启动

需要已启动的 Docker Engine 和支持 `--wait` 的 Docker Compose v2，且本机 8080 端口可用。在项目根目录运行：

```bash
docker compose up --build --wait
```

打开 [http://localhost:8080](http://localhost:8080)。首次启动会建立数据库结构并填入示例内容；再次执行不会重复生成示例文章。仅 Web 服务绑定本机地址，MySQL 和 API 不向宿主机开放端口。

默认配置无需创建 `.env`。如需修改端口或演示密码，可复制 `.env.example` 为 `.env` 并同时保持 `PUBLIC_PORT` 与 `PUBLIC_ORIGIN` 一致；已经生成的演示账号密码不会因修改配置而自动重置。

| 演示身份 | 邮箱 | 密码 |
| --- | --- | --- |
| 管理员 | `admin@blog.local` | `Demo12345!` |
| 博主一 | `author1@blog.local` | `Demo12345!` |
| 博主二 | `author2@blog.local` | `Demo12345!` |

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
