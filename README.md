# DCST

Docker 化的 Cloudflare 优选 IP 工具。后台持续自动测速，Web 控制台管理参数，结果 API 始终只返回当前最强的 20 个 IP。

> [!TIP]
> 国内网络可使用自建 GitHub 加速器：
> [加速访问仓库](https://github.i3.pub/https://github.com/clover-eric/dcst) ·
> [加速下载标准 Compose](https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/dcst/main/docker-compose.yml) ·
> [加速下载飞牛 Compose](https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/dcst/main/docker-compose.fnos.yml) ·
> [加速下载整仓 ZIP](https://github.i3.pub/https://github.com/clover-eric/dcst/archive/refs/heads/main.zip)

## 功能

- 基于多流并行、慢启动预热和稳态计量的 CFST 优化引擎。
- 自动读取 Cloudflare 官方最新 IPv4/IPv6 网段，接口失败时回退到内置列表。
- 默认每 60 分钟自动测速，无需外部 cron。
- Windows、Linux、macOS、NAS、软路由均可通过 Docker 使用。
- 自动适配 `linux/amd64`、`linux/arm64`、`linux/arm/v7`。
- 测量更多候选后，只持久化和公开按下载吞吐排序的前 20 个 IP。
- 内置响应式 Web 管理页、手动测速和参数修改。
- 首次访问设置 8 位数字管理员密码；密码使用 bcrypt 哈希保存。
- Web 管理使用 HttpOnly 会话，修改密码后旧会话自动失效。
- Web 管理页支持一键拉取最新镜像并自动重建容器。
- 配置和结果存放在 Docker 数据卷中，重启不丢失。

## 快速部署

只需要 Docker Compose：

```bash
mkdir dcst
cd dcst
curl -fL -O https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/dcst/main/docker-compose.yml
docker compose up -d
```

打开：

```text
http://你的NAS或路由器IP:10123/
```

如果 GHCR 镜像尚未发布，或者需要从源码构建：

```bash
git clone https://github.i3.pub/https://github.com/clover-eric/dcst.git
cd dcst
docker compose up -d --build
```

也可以使用国内加速安装脚本，一次性下载源码、构建并启动：

```bash
curl -fL \
  https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/dcst/main/scripts/install-cn.sh \
  -o install-cn.sh
bash install-cn.sh
```

自定义安装目录：

```bash
DCST_INSTALL_DIR=/vol1/1000/docker/dcst bash install-cn.sh
```

`i3.pub` 加速 GitHub 仓库、Raw 文件和源码包，但不代理 GHCR。GHCR 拉取较慢时，可先按上面的加速方式下载源码并执行 `docker compose up -d --build`，或者把 `DCST_IMAGE` 改成自己的 GHCR 镜像仓库地址。

默认端口是 `10123`。如需修改：

```bash
DCST_PORT=10124 docker compose up -d
```

## 飞牛 fnOS 专属部署

仓库提供专用文件：

[docker-compose.fnos.yml](https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/dcst/main/docker-compose.fnos.yml)

它针对飞牛做了以下调整：

- 默认使用 `/vol1/1000/docker/dcst/data` 持久化配置和结果。
- 默认端口为 `10123`，减少与 NAS 常用服务冲突。
- `restart: always`，适合 24 小时运行。
- 提高文件句柄上限，支持高并发测速。
- 限制 Docker 日志为 3 × 10 MB，避免长期运行占满系统盘。
- 非 root、只保留必要权限，并启用容器健康检查。

快速步骤：

1. 在飞牛文件管理器的“管理员视角”创建 `/vol1/1000/docker/dcst/data`。
2. 打开“Docker → Compose → 新增项目”。
3. 项目名填 `dcst`，路径选择 `/vol1/1000/docker/dcst`。
4. 将 `docker-compose.fnos.yml` 内容粘贴到 Compose 编辑器并启动。
5. 访问 `http://飞牛IP:10123/`。

如果权限提示无法写入数据目录，在飞牛 SSH 中执行：

```bash
sudo mkdir -p /vol1/1000/docker/dcst/data
sudo chown -R 1000:1000 /vol1/1000/docker/dcst/data
```

完整步骤见 [飞牛 NAS 部署指南](docs/fnos.md)。

## 管理员密码

首次部署后打开 Web 页面，会显示初始化窗口：

1. 设置一个 **8 位纯数字密码**；
2. 再次输入确认；
3. 初始化完成后自动登录管理界面。

密码要求：

- 必须正好 8 位；
- 只能包含数字；
- 禁止 8 位完全相同或明显的连续数字序列；
- 连续登录失败会触发 5 分钟限流。

密码经过 bcrypt 哈希后保存到 `/data/auth.json`，不会保存明文。登录状态使用
24 小时 HttpOnly 会话 Cookie。登录后可在配置区域点击“修改密码”，修改成功后
其他旧会话全部失效。

首次初始化前不要把未初始化的管理页面直接暴露到公网。升级自旧版本的用户首次
打开页面同样需要设置密码；为了兼容已有自动化脚本，显式设置 `ADMIN_TOKEN` 时
仍允许通过 Bearer Token 调用管理 API，但 Web 管理不再要求或展示令牌。

## 结果 API

JSON，默认接口：

```text
http://你的域名或IP:10123/result
```

示例：

```json
{
  "ready": true,
  "running": false,
  "updated_at": "2026-10-23T16:00:00+08:00",
  "count": 20,
  "results": [
    {
      "ip": "104.16.1.2",
      "sent": 3,
      "received": 3,
      "loss_rate": 0,
      "latency_ms": 24.5,
      "download_mb_s": 12.34,
      "region": "HKG"
    }
  ]
}
```

其他格式：

```text
/result?format=csv
/result?format=text
```

`/result` 已启用允许跨域读取的 CORS 响应头，可直接供浏览器插件、DDNS 脚本和其他第三方应用调用。

常用接口：

| 路径 | 方法 | 用途 |
|---|---|---|
| `/` | GET | Web 管理页面 |
| `/result` | GET | 前 20 个结果 JSON |
| `/result?format=csv` | GET | CSV |
| `/result?format=text` | GET | 每行一个 IP |
| `/api/status` | GET | 当前运行状态 |
| `/api/config` | GET | 当前配置 |
| `/api/config` | POST | 修改配置，需要管理员登录 |
| `/api/run` | POST | 立即测速，需要管理员登录 |
| `/api/auth/status` | GET | 密码是否初始化及当前登录状态 |
| `/api/setup` | POST | 首次设置密码 |
| `/api/login` | POST | 密码登录 |
| `/api/logout` | POST | 退出登录 |
| `/api/password` | POST | 修改密码 |
| `/api/update` | POST | 一键更新容器，需要管理员登录 |
| `/health` | GET | 容器健康检查 |

## 排序规则

每轮测速会测试比最终结果更多的候选 IP，然后按以下顺序排序并截取前 20 个：

1. 下载速度从高到低；
2. 下载速度相同时，丢包率从低到高；
3. 再按平均延迟从低到高。

数据卷中不会保留超过 20 个结果。若某一轮有效候选不足 20 个，则返回实际可用数量，不会用虚假记录补足。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---:|---|
| `DCST_PORT` | `10123` | 宿主机端口 |
| `INTERVAL_MINUTES` | `60` | 自动测速间隔 |
| `CANDIDATES` | `40` | 每轮下载测试候选数 |
| `STREAMS` | `16` | 每候选 IP 并发连接数 |
| `TEST_DURATION_SECONDS` | `5` | 单 IP 测速时间 |
| `WARMUP_SECONDS` | `1` | 排除慢启动的时间 |
| `TEST_PORT` | `443` | 测速端口 |
| `TEST_URL` | 公共地址 | 下载测速 URL |
| `MIN_SPEED_MB_S` | `0` | 最低下载速度过滤 |
| `MAX_LATENCY_MS` | `9999` | 最大平均延迟 |
| `MAX_LOSS_RATE` | `1` | 最大丢包率，范围 0–1 |
| `USE_IPV6` | `true` | 是否包含 IPv6 |
| `RUN_ON_START` | `true` | 容器启动后立即测速 |
| `ADMIN_TOKEN` | 未设置 | 可选的旧版管理 API 兼容令牌 |
| `DCST_IMAGE` | GHCR 镜像 | 可替换为国内镜像地址 |
| `UPDATE_API_TOKEN` | 内部默认值 | DCST 与更新代理之间的内部令牌 |
| `WATCHTOWER_IMAGE` | `nickfedor/watchtower:1.23.2` | 一键更新代理镜像 |

从源码构建时还可通过 `GO_IMAGE` 和 `ALPINE_IMAGE` 替换 Docker 基础镜像。

## 一键在线更新

Compose 中内置了独立的 `updater` 服务，使用活跃维护的
`nickfedor/watchtower`。登录 Web 管理后点击“检查并更新”：

1. DCST 通过内部网络调用更新代理；
2. 更新代理拉取当前容器使用的镜像标签；
3. 有新镜像时自动重建 DCST 容器；
4. 密码、配置和测速结果都保存在 `/data` 中，不会丢失。

更新代理没有对宿主机开放端口，且只处理带有
`com.centurylinklabs.watchtower.enable=true` 标签的 DCST 容器。它需要访问
`/var/run/docker.sock`，属于容器管理的高权限能力，因此建议保持内部网络隔离，
并修改默认 `UPDATE_API_TOKEN`。

从 `v1.1.x` 升级到 `v1.2.x` 时，需要先使用新版 `docker-compose.yml` 重新创建
一次项目，因为旧镜像本身还没有 Web 更新接口，也无法自带更新代理。升级到
`v1.2.0` 后，后续版本即可直接使用页面上的“检查并更新”按钮。

Web 页面修改的配置会写入 `/data/config.json`，后续重启优先使用已保存配置。

## 高带宽测速

公共测速地址可能限速或触发反滥用策略。要达到真实高带宽，建议在 Cloudflare 后自建至少 1 GiB 的测速文件：

```env
TEST_URL=https://speed.example.com/2GB.bin
TEST_PORT=443
STREAMS=16
```

如果测速地址不是 HTTPS 443，`TEST_PORT` 必须与实际端口一致。

## 反向代理

建议通过 Nginx、Caddy、Traefik 或 NAS 自带反向代理提供 HTTPS：

```nginx
location / {
    proxy_pass http://127.0.0.1:10123;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

如果 Web 管理端暴露到公网，请先完成密码初始化并设置不易猜测的 8 位数字密码，
同时建议在反向代理外层增加 IP 白名单、身份认证或其他访问控制。

## 数据与升级

```bash
docker compose pull
docker compose up -d
```

数据保存在命名卷 `dcst-data` 中，包括：

```text
/data/config.json
/data/result.json
/data/auth.json
/data/admin_token  # 仅旧版兼容令牌存在时
```

备份该卷即可完整保存配置与结果。

## 本地开发

```bash
go test -race ./...
cd engine && go test -race ./...
go build ./cmd/dcst
```

## 许可证

本项目管理代码使用 MIT License。内置测速引擎来自 `XIU2/CloudflareSpeedTest`，其许可证与来源见 `THIRD_PARTY_NOTICES.md` 和 `engine/LICENSE`。
