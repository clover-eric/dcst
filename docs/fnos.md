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
http://飞牛IP:10123/
```

如果 `10123` 已被占用，可以修改环境变量：

```yaml
DCST_PORT: "10124"
```

也可以将 Compose 中的 `${DCST_PORT:-10123}:10123` 直接改成其他宿主机端口；冒号右侧的容器端口需要与容器的监听端口保持一致。

## 5. 初始化管理员密码

首次打开：

```text
http://飞牛IP:10123/
```

页面会自动显示初始化窗口。设置一个 8 位纯数字密码并再次确认，完成后会自动
登录管理界面。密码不会保存明文，哈希文件位于：

```text
/vol1/1000/docker/dcst/data/auth.json
```

登录后可以手动测速、修改参数、修改密码，也可以点击“检查并更新”触发容器
在线升级。`/result` 结果 API 始终是公开只读接口。

建议在首次初始化完成前，不要将管理页面直接暴露到公网。

一键升级由 `dcst-updater` 辅助容器完成。该容器只在 Docker 内部网络提供更新
接口，不需要映射到宿主机端口；它需要读取 `/var/run/docker.sock`，请不要把
Docker Socket 或 `UPDATE_API_TOKEN` 暴露给外部网络。

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

配置、结果和管理员密码哈希均保存在数据目录中，更新镜像不会丢失。
