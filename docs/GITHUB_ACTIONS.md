# GitHub Actions：持续集成与镜像发布

本开源仓库只负责验证代码和交付可复用的容器镜像，不连接特定服务器。需要部署时，由使用者在自己的环境中拉取指定版本镜像并运行 Compose。

## 工作流

| 触发 | 执行内容 |
| --- | --- |
| Push、Pull Request | Go 格式、测试和静态检查；前端 lint 与构建；使用真实 MySQL 的 Compose 集成测试。 |
| `v1.2.3` 等版本标签 | 上述检查通过后，发布 API 和 Web 镜像到 GHCR，支持 `linux/amd64`、`linux/arm64`。 |

镜像地址按仓库名自动生成：

```text
ghcr.io/<owner>/<repo>-api:<tag>
ghcr.io/<owner>/<repo>-web:<tag>
```

发布任务只在版本标签上运行，且必须等待测试成功。它仅使用 GitHub 自动提供的 `GITHUB_TOKEN`，仓库无需配置 SSH 密钥、服务器地址或数据库密码。普通 Pull Request 不会获得镜像发布权限。

## 发布版本

维护者在确认版本内容后推送标签：

```bash
git tag v1.0.0
git push origin v1.0.0
```

Actions 的 **CI and image release** 工作流成功后，可在 GitHub Packages 中查看镜像。GHCR 包初次发布后可能默认为私有；如希望开源用户免登录拉取，需要在包设置中将 API、Web 两个包设为公开。已发布标签应保持不变；修订内容使用新版本号。

## 使用已发布镜像

[`compose.release.yaml`](../compose.release.yaml) 是通用的镜像运行示例。按自己的环境设置 `IMAGE_PREFIX`、`IMAGE_TAG`、数据库密码、演示账号密码和 `PUBLIC_ORIGIN`，再执行：

```bash
docker compose -f compose.release.yaml pull
docker compose -f compose.release.yaml up -d --wait
```

例如本仓库的 `IMAGE_PREFIX` 为 `ghcr.io/goodgoodstudy-stone/blog-system`，`IMAGE_TAG` 为 `v1.0.0`。`PUBLIC_ORIGIN` 必须与浏览器访问地址一致；默认只绑定本机 `127.0.0.1:8080`。运行参数可放在被 `.gitignore` 排除的 `.env` 文件中。若 GHCR 包保持私有，拉取前需由运行方自行登录 GHCR。

特定服务器的地址、SSH 凭据、域名和自动更新策略由部署方单独管理，不进入此公开仓库的 Actions 工作流。
