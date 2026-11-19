#!/usr/bin/env bash
# Hostower 一键安装（也用于升级）：
#
#   bash <(curl -fsSL https://raw.githubusercontent.com/tsienwan/hostower/main/install.sh)
#
# 它做的事情都很直白，先读一遍再运行也不费事：
#   1. 检查环境：Linux、root（或 sudo）、Docker 和 Docker Compose v2；
#   2. 找到要装的版本（默认是最新的正式版本），从那个版本的标签下载对应的 compose 文件；
#   3. 在安装目录（默认 /opt/hostower）建好 data/ 和 config/，用 docker compose 启动；
#   4. 第一次安装时打印访问地址和初始密码。
# 再运行一次就是升级：换成新版本的 compose 文件（旧的留一份 .bak），data/ 原样保留。
#
# 用法：install.sh [版本] [--host] [--dir 目录] [--url 对外地址]
#   版本       比如 v0.1.2；不写就装最新的正式版本
#   --host     装宿主机版：还会把这台机器作为内置的 local 主机来管理（特权容器，等于这台机器上的 root）
#   --dir      安装目录，默认 /opt/hostower
#   --url      对外访问地址（放在反向代理之后时），比如 https://hostower.example.com
# 环境变量 GITHUB_TOKEN：仓库或发布页不公开时用它访问 GitHub。
set -euo pipefail

REPO="tsienwan/hostower"
VERSION=""
FLAVOR="default"
DIR="/opt/hostower"
PUBLIC_URL=""

# 界面语言跟随系统：中文环境说中文，其余说英文。
case "${LC_ALL:-${LANG:-}}" in zh*) ZH=1 ;; *) ZH=0 ;; esac
say() { if [ "$ZH" = 1 ]; then printf '%s\n' "$1"; else printf '%s\n' "$2"; fi; }
die() { say "错误：$1" "Error: $2" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --host) FLAVOR="host" ;;
    --dir) shift; DIR="${1:?}" ;;
    --url) shift; PUBLIC_URL="${1:?}" ;;
    -h|--help) sed -n '2,20p' "$0" 2>/dev/null || true; exit 0 ;;
    v[0-9]*) VERSION="$1" ;;
    *) die "不认识的参数：$1" "unknown argument: $1" ;;
  esac
  shift
done

# ---- 1. 环境 ----
[ "$(uname -s)" = "Linux" ] || die "只支持 Linux" "Linux only"
SUDO=""
if [ "$(id -u)" != 0 ]; then
  command -v sudo >/dev/null 2>&1 || die "需要 root 或 sudo" "root or sudo is required"
  SUDO="sudo"
fi
command -v curl >/dev/null 2>&1 || die "需要 curl" "curl is required"
if ! command -v docker >/dev/null 2>&1; then
  say "没有找到 Docker。可以按官方文档安装：https://docs.docker.com/engine/install/" \
      "Docker was not found. Install it following https://docs.docker.com/engine/install/"
  exit 1
fi
$SUDO docker compose version >/dev/null 2>&1 || die "需要 Docker Compose v2（docker compose 命令）" "Docker Compose v2 (the docker compose command) is required"
case "$PUBLIC_URL" in ""|https://*|http://*) ;; *) die "--url 要以 https:// 开头" "--url must start with https://" ;; esac

gh_curl() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    curl -fsSL -H "Authorization: Bearer $GITHUB_TOKEN" "$@"
  else
    curl -fsSL "$@"
  fi
}

# ---- 2. 版本 ----
if [ -z "$VERSION" ]; then
  VERSION="$(gh_curl "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\(v[0-9][^"]*\)".*/\1/p' | head -n 1)" || true
  [ -n "$VERSION" ] || die "查不到最新版本（网络不通，或者仓库还没公开）" "could not find the latest release (network problem, or the repository is not public yet)"
fi
case "$VERSION" in v[0-9]*.[0-9]*.[0-9]*) ;; *) die "版本号格式不对：$VERSION" "bad version: $VERSION" ;; esac
COMPOSE="compose.yaml"
[ "$FLAVOR" = host ] && COMPOSE="compose.host.yaml"
say "安装 Hostower $VERSION（$([ "$FLAVOR" = host ] && echo 宿主机版 || echo 默认镜像)）到 $DIR" \
    "Installing Hostower $VERSION ($FLAVOR image) into $DIR"

# ---- 3. 文件和目录 ----
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
gh_curl "https://raw.githubusercontent.com/$REPO/$VERSION/deploy/$COMPOSE" -o "$TMP" ||
  die "下载 $COMPOSE 失败" "failed to download $COMPOSE"
grep -q "ghcr.io/$REPO:" "$TMP" || die "下载到的 compose 文件不对" "the downloaded compose file looks wrong"
if [ -n "$PUBLIC_URL" ]; then
  sed -i "s#HOSTOWER_PUBLIC_URL: .*#HOSTOWER_PUBLIC_URL: $PUBLIC_URL#" "$TMP"
fi

$SUDO mkdir -p "$DIR/data" "$DIR/config"
FIRST=1
[ -s "$DIR/data/hostower.db" ] && FIRST=0
if [ -f "$DIR/compose.yaml" ]; then
  # 升级时保留原来的对外地址（没有用 --url 另外指定时）。
  if [ -z "$PUBLIC_URL" ]; then
    OLD_URL="$(sed -n 's/^[[:space:]]*HOSTOWER_PUBLIC_URL: \(.*\)$/\1/p' "$DIR/compose.yaml" | head -n 1)"
    [ -n "$OLD_URL" ] && sed -i "s#HOSTOWER_PUBLIC_URL: .*#HOSTOWER_PUBLIC_URL: $OLD_URL#" "$TMP"
  fi
  $SUDO cp "$DIR/compose.yaml" "$DIR/compose.yaml.bak"
fi
$SUDO install -m 0644 "$TMP" "$DIR/compose.yaml"
# 默认镜像以 UID 65532 运行，要能写 data/；宿主机版以 root 运行。
[ "$FLAVOR" = default ] && $SUDO chown 65532:65532 "$DIR/data"

# ---- 4. 启动 ----
(cd "$DIR" && $SUDO docker compose up -d) || die "启动失败，用 docker compose logs 查看原因" "failed to start; check docker compose logs"

URL="$(sed -n 's/^[[:space:]]*HOSTOWER_PUBLIC_URL: \(.*\)$/\1/p' "$DIR/compose.yaml" | head -n 1)"
if [ "$FIRST" = 1 ]; then
  for _ in $(seq 1 30); do
    $SUDO test -s "$DIR/data/initial-password" && break
    sleep 1
  done
  PASS="$($SUDO cat "$DIR/data/initial-password" 2>/dev/null || true)"
  echo
  say "Hostower 已经启动。" "Hostower is running."
  say "  地址：  $URL" "  URL:      $URL"
  say "  用户名：admin" "  User:     admin"
  say "  密码：  ${PASS:-（还没生成，稍后运行 sudo cat $DIR/data/initial-password）}" \
      "  Password: ${PASS:-(not ready yet; run sudo cat $DIR/data/initial-password later)}"
  echo
  say "它只监听本机的 127.0.0.1。在远程服务器上，先在自己的电脑上建一条 SSH 隧道，再打开上面的地址：" \
      "It listens on 127.0.0.1 only. On a remote server, open an SSH tunnel from your computer first, then browse to the URL above:"
  echo "  ssh -L 18443:127.0.0.1:18443 <user>@<server>"
  # shellcheck disable=SC1111 # 中文引号是有意的
  say "登录后请立刻修改密码，并在“设置 → 安全”里添加 Passkey 或动态码。" \
      "After signing in, change the password and add a Passkey or authenticator under Settings → Security."
else
  say "Hostower 已更新到 $VERSION，数据保留不变（旧的 compose 文件在 compose.yaml.bak）。" \
      "Hostower has been updated to $VERSION; your data is unchanged (the old compose file is in compose.yaml.bak)."
fi
echo
say "常用命令（在 $DIR 目录下）：docker compose ps | docker compose logs -f | docker compose down" \
    "Useful commands (in $DIR): docker compose ps | docker compose logs -f | docker compose down"
