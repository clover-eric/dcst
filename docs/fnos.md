# 飞牛 fnOS 部署指南

DCST 的飞牛专用 Compose 文件：

```text
https://github.i3.pub/https://raw.githubusercontent.com/clover-eric/dcst/main/docker-compose.fnos.yml
```

## 1. 确认 Docker

在飞牛应用中心确认 Docker 已安装并开启。第一次使用 Docker 时，按飞牛提示选择镜像和容器存储空间。

## 2. 创建数据目录

飞牛通常把第一个用户的配置放在 `/vol1/1000`。使用文件管理器的“管理员视角”创建：

```text
/vol1/1000/docker/dcst/data
```

也可以 SSH 执行：

```bash
sudo mkdir -p /vol1/1000/docker/dcst/data
sudo chown -R 1000:1000 /vol1/1000/docker/dcst/data
```

若数据存放在其他存储池或其他用户下，请把 Compose 中 `DCST_DATA_DIR` 的默认值改成实际路径。

## 3. 新建 Compose 项目

1. 打开飞牛的 Docker 管理界面。
2. 进入“Compose”并点击“新增项目”。
3. 项目名称填写 `dcst`。
4. 保存路径选择 `/vol1/1000/docker/dcst`。
5. 选择创建 `docker-compose.yml`。
6. 粘贴仓库中的 `docker-compose.fnos.yml` 内容。
7. 勾选“创建项目后立即启动”。

## 4. 访问 Web 管理页

```text
http://飞牛IP:18080/
```

如果 `18080` 已被占用，可以修改环境变量：

```yaml
DCST_PORT: "28080"
```

也可以将 Compose 中的 `${DCST_PORT:-18080}:8080` 直接改成其他宿主机端口，冒号右侧的 `8080` 不要修改。

## 5. 获取管理员令牌

未设置 `ADMIN_TOKEN` 时，首次运行会自动生成令牌：

```bash
docker logs dcst 2>&1 | grep "管理员令牌"
```

令牌会保存在：

```text
/vol1/1000/docker/dcst/data/admin_token
```

登录 Web 页面后可以手动测速和修改配置。`/result` 结果 API 是公开只读接口。

## 6. 国内网络与镜像拉取

飞牛的 Docker 镜像无法通过 `github.i3.pub` 加速，因为该加速器代理的是 GitHub 文件和仓库，不代理 GHCR。

如果 GHCR 拉取正常，直接启动即可。如果拉取缓慢：

### 方案 A：使用国内镜像仓库

先把 `ghcr.io/clover-eric/dcst:latest` 同步到自己的镜像仓库，再修改：

```yaml
DCST_IMAGE: "你的镜像仓库/dcst:latest"
```

### 方案 B：通过加速源码在飞牛上构建

```bash
cd /vol1/1000/docker
curl -fL \
  https://github.i3.pub/https://github.com/clover-eric/dcst/archive/refs/heads/main.zip \
  -o dcst.zip
unzip dcst.zip
cd dcst-main
docker compose -f docker-compose.fnos.yml up -d --build
```

如果 Docker Hub 基础镜像也不稳定，可在构建时指定自己的镜像代理：

```bash
GO_IMAGE=你的代理/golang:1.27-alpine \
ALPINE_IMAGE=你的代理/alpine:3.22 \
docker compose -f docker-compose.fnos.yml up -d --build
```

## 7. 推荐设置

24 小时运行时建议：

```yaml
INTERVAL_MINUTES: "60"
CANDIDATES: "40"
STREAMS: "16"
TEST_DURATION_SECONDS: "5"
RUN_ON_START: "true"
```

测速期间会产生较多连接，飞牛设备性能较弱时，可把 `STREAMS` 降到 `8`，把 `CANDIDATES` 降到 `20`。

## 8. 更新

```bash
cd /vol1/1000/docker/dcst
docker compose pull
docker compose up -d
```

使用本地构建时：

```bash
docker compose -f docker-compose.fnos.yml up -d --build
```

配置、结果和管理员令牌均保存在数据目录中，更新镜像不会丢失。
