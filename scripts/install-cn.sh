#!/usr/bin/env bash

set -euo pipefail

ACCELERATOR="${DCST_GITHUB_ACCELERATOR:-https://github.i3.pub/}"
ACCELERATOR="${ACCELERATOR%/}/"
REPOSITORY="https://github.com/clover-eric/dcst"
INSTALL_DIR="${DCST_INSTALL_DIR:-$PWD/dcst}"

if ! command -v curl >/dev/null 2>&1; then
  echo "错误：需要安装 curl。"
  exit 1
fi
if ! command -v unzip >/dev/null 2>&1; then
  echo "错误：需要安装 unzip。"
  exit 1
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "错误：需要先安装 Docker。"
  exit 1
fi
if [ -e "$INSTALL_DIR" ] && [ "$(find "$INSTALL_DIR" -mindepth 1 -maxdepth 1 2>/dev/null | head -n 1)" ]; then
  echo "错误：目录已存在且不为空：$INSTALL_DIR"
  echo "请设置 DCST_INSTALL_DIR 指定新的安装目录。"
  exit 1
fi

workdir=$(mktemp -d "${TMPDIR:-/tmp}/dcst.XXXXXX")
trap 'rm -rf "$workdir"' EXIT

archive_url="${ACCELERATOR}${REPOSITORY}/archive/refs/heads/main.zip"
echo "正在通过国内加速器下载源码：$archive_url"
curl -fL --retry 4 --retry-all-errors --connect-timeout 10 \
  "$archive_url" -o "$workdir/dcst.zip"
unzip -q "$workdir/dcst.zip" -d "$workdir/source"

source_dir=$(find "$workdir/source" -mindepth 1 -maxdepth 1 -type d | head -n 1)
if [ -z "$source_dir" ]; then
  echo "错误：下载包中没有找到源码目录。"
  exit 1
fi

mkdir -p "$INSTALL_DIR"
cp -a "$source_dir/." "$INSTALL_DIR/"

echo "正在构建并启动 DCST：$INSTALL_DIR"
cd "$INSTALL_DIR"
docker compose up -d --build

echo
echo "部署完成：http://$(hostname -I 2>/dev/null | awk '{print $1}' || echo 'NAS-IP'):10123/"
echo "查看管理员令牌："
echo "  docker logs dcst 2>&1 | grep \"管理员令牌\""
