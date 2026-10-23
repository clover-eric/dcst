# DCST

Docker 化的 Cloudflare 优选 IP 工具。后台持续自动测速，Web 控制台管理参数，结果 API 始终只返回当前最强的 20 个 IP。

## 功能

- 基于多流并行、慢启动预热和稳态计量的 CFST 优化引擎。
- 自动读取 Cloudflare 官方最新 IPv4/IPv6 网段，接口失败时回退到内置列表。
- 默认每 60 分钟自动测速，无需外部 cron。
- Windows、Linux、macOS、NAS、软路由均可通过 Docker 使用。
- 自动适配 `linux/amd64`、`linux/arm64`、`linux/arm/v7`。
- 测量更多候选后，只持久化和公开按下载吞吐排序的前 20 个 IP。
- 内置响应式 Web 管理页、手动测速和参数修改。
- 管理接口使用独立令牌；公开结果 API 只读。
- 配置和结果存放在 Docker 数据卷中，重启不丢失。

## 快速部署

只需要 Docker Compose：

```bash
mkdir dcst
cd dcst
curl -O https://raw.githubusercontent.com/clover-eric/dcst/main/docker-compose.yml
docker compose up -d
```

打开：

```text
http://你的NAS或路由器IP:8080/
```

如果 GHCR 镜像尚未发布，或者需要从源码构建：

```bash
git clone https://github.com/clover-eric/dcst.git
cd dcst
docker compose up -d --build
```

默认端口是 `8080`。如需修改：

```bash
DCST_PORT=18080 docker compose up -d
```

## 管理员令牌

未设置 `ADMIN_TOKEN` 时，首次启动会生成随机令牌并输出到日志：

```bash
docker logs dcst 2>&1 | grep "管理员令牌"
```

也可以在 Compose 中固定：

```yaml
environment:
  ADMIN_TOKEN: "请替换为足够长的随机字符串"
```

令牌用于“立即测速”和修改配置。查看结果与 Web 首页不需要令牌。

## 结果 API

JSON，默认接口：

```text
http://你的域名或IP:8080/result
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
| `/api/config` | POST | 修改配置，需要管理员令牌 |
| `/api/run` | POST | 立即测速，需要管理员令牌 |
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
| `DCST_PORT` | `8080` | 宿主机端口 |
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
| `ADMIN_TOKEN` | 自动生成 | 管理接口令牌 |

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
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

如果 Web 管理端暴露到公网，请务必设置强随机 `ADMIN_TOKEN`，并建议在外层增加访问控制。

## 数据与升级

```bash
docker compose pull
docker compose up -d
```

数据保存在命名卷 `dcst-data` 中，包括：

```text
/data/config.json
/data/result.json
/data/admin_token
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
