# GitHub Actions：测试、发布与演示服务器部署

## 触发规则

- 每次 push 和 pull request：执行 Go 测试与静态检查、前端检查，以及真实 MySQL + HTTP 的 Compose 集成测试。
- 推送 `v1.2.3` 形式的版本标签：上述检查通过后，发布 `linux/amd64` 和 `linux/arm64` 的 API、Web 镜像到 GHCR。
- 仓库变量 `DEPLOY_ENABLED=true` 时：镜像发布成功后，通过 SSH 在演示服务器上拉取镜像并执行 `docker compose up -d --wait`。部署失败会使工作流失败。

镜像名为 `ghcr.io/<owner>/<repo>-api:<tag>` 和 `ghcr.io/<owner>/<repo>-web:<tag>`。发布使用 GitHub 内置的 `GITHUB_TOKEN`，不需要个人访问令牌。版本标签指向的代码是发布内容；不要移动已经发布的标签。

## 服务器准备

服务器需要 Linux、Docker Engine、支持 `--wait` 的 Docker Compose v2。部署用户须能通过 SSH 登录并运行 `docker compose`，且其主目录可写。部署文件保存在该用户的 `~/blog-system/`，数据库和图片保存在名为 `blog-engineering-release` 的 Compose 数据卷中。工作流不会清空这些数据卷。

默认 Web 只绑定服务器的 `127.0.0.1:8080`。如需通过域名访问，使用服务器已有的 HTTPS 反向代理转发到该地址，并将 `DEPLOY_PUBLIC_ORIGIN` 设为浏览器实际访问的来源，例如 `https://blog.example.com`。如需直接访问服务器端口，可把 `DEPLOY_PUBLIC_BIND` 设为 `0.0.0.0`，并确保 `DEPLOY_PUBLIC_ORIGIN` 与访问地址完全一致。这个项目仍按演示用途配置，不作为正式运营部署模板。

## GitHub 配置

在仓库 Settings → Secrets and variables → Actions 中设置仓库变量 `DEPLOY_ENABLED=true`。创建名为 `demo` 的 Environment，并在该 Environment 下设置：

| 类型 | 名称 | 内容 |
| --- | --- | --- |
| Secret | `DEPLOY_HOST` | 服务器域名或 IPv4 地址。 |
| Secret | `DEPLOY_USER` | 可运行 Docker Compose 的 SSH 用户名。 |
| Secret | `DEPLOY_SSH_KEY` | 对应 SSH 私钥的完整内容。 |
| Secret | `DEPLOY_KNOWN_HOSTS` | 已核对指纹的服务器 SSH host key 记录；端口非 22 时使用 `[host]:port` 格式。 |
| Secret | `DEPLOY_MYSQL_ROOT_PASSWORD` | 稳定的数据库 root 密码。 |
| Secret | `DEPLOY_MYSQL_PASSWORD` | 稳定的博客数据库密码。 |
| Secret | `DEPLOY_DEMO_PASSWORD` | 首次创建演示账号时使用的密码。 |
| Variable | `DEPLOY_PUBLIC_ORIGIN` | 浏览器实际访问的来源，含 `http://` 或 `https://`，不带路径和末尾斜杠。 |
| Variable | `DEPLOY_SSH_PORT` | 可选；默认 `22`。 |
| Variable | `DEPLOY_PUBLIC_BIND` | 可选；默认 `127.0.0.1`，也可设 `0.0.0.0`。 |
| Variable | `DEPLOY_PUBLIC_PORT` | 可选；默认 `8080`。 |

三个密码至少 16 位，只使用英文字母、数字、下划线和连字符。首次部署后保持数据库密码不变；仅修改环境变量不会更改已有 MySQL 数据卷中的账号密码。演示账号也只在首次初始化时创建，之后修改 `DEPLOY_DEMO_PASSWORD` 不会重设其密码。

SSH host key 应从可信渠道核对，再填写 `DEPLOY_KNOWN_HOSTS`。部署脚本严格检查该记录，不自动信任未知主机。`demo` Environment 可设置 GitHub 审批规则，让实际部署在批准后运行。

## 发布

将项目推送到 GitHub 仓库，并推送版本标签，例如：

```bash
git tag v1.0.0
git push origin v1.0.0
```

查看 Actions 中的 `CI/CD` 工作流。`test`、`publish` 和 `deploy` 依次成功后，访问 `DEPLOY_PUBLIC_ORIGIN` 验证页面。若 `DEPLOY_ENABLED` 尚未设为 `true`，`deploy` 会跳过；镜像仍会发布。GitHub Packages 的可见性由仓库所有者管理，默认可能是私有；部署时工作流会临时登录 GHCR 拉取镜像，随后退出登录。

如需手动使用已发布镜像，可配置 `compose.release.yaml` 所需环境变量后运行：

```bash
docker compose -f compose.release.yaml pull
docker compose -f compose.release.yaml up -d --wait
```

服务器部署脚本位于 [`deploy/deploy.sh`](../deploy/deploy.sh)，便于审查 SSH、环境变量和 Compose 操作。
