# 常见问题

**简体中文** | [繁體中文](faq.zh-TW.md) | [English](faq.en.md) | [日本語](faq.ja.md) | [한국어](faq.ko.md) | [Français](faq.fr.md) | [Deutsch](faq.de.md)

接入主机、装 Docker、装应用这几步里最常遇到的问题和注意事项。示例里的主机名、地址都是占位符，换成你自己的。

- [接入主机](#接入主机)
- [sudo](#sudo)
- [Docker 和容器应用](#docker-和容器应用)
- [增强采集](#增强采集)
- [更新 Hostower](#更新-hostower)

---

## 接入主机

### 为什么 SSH 凭据不能用 root？

Hostower 等于“所有主机的钥匙”，所以在主机这边还要有一道它自己管不到的门：

1. **面板失守时控制损失**：数据库和主密钥一旦泄露，攻击者能拿到 Hostower 存的私钥。root 的私钥意味着直接接管整台主机；受限账号只能做 sudo 允许的事。
2. **判级不可能完美**：判级引擎要解析 shell，总可能有没想到的绕过写法。sudo 的限制由主机系统执行，是独立的第二道保险。
3. **交互终端不逐条判级**：开终端要按 L2 批准、全程录像，但里面敲的命令不再逐条判级。用 root 登录，一旦批准就什么都能做。
4. **运维习惯**：很多系统默认禁止 root 用 SSH 登录；用专用账号登录，主机的 `auth.log` 和 sudo 日志里能分清哪些是 Hostower 做的。

这是硬性限制：用户名填 `root` 会直接被拒绝。

### 怎么给 Hostower 建一个专用账号？

在被管机上用 root 执行：

```bash
useradd -m -s /bin/bash hostower
mkdir -p /home/hostower/.ssh
echo 'ssh-ed25519 AAAA…（公钥）' >> /home/hostower/.ssh/authorized_keys
chown -R hostower:hostower /home/hostower/.ssh
chmod 700 /home/hostower/.ssh
chmod 600 /home/hostower/.ssh/authorized_keys
```

然后按下一节配 sudo，再在 Hostower 的“凭据”里添加 SSH 凭据：用户名填 `hostower`，私钥填和上面公钥配对的那一把（从 `-----BEGIN` 到 `-----END` 整段）。

### root 已经能用密钥登录了，新账号还要再配密钥吗？

要。公钥是按账号放的：每个账号只认自己家目录里 `~/.ssh/authorized_keys` 的公钥，`/root/.ssh/authorized_keys` 只对 root 有效。

两种做法：

- **推荐：给 Hostower 单独生成一把密钥**。root 继续用你原来的密钥，那把私钥只留在你自己手里，不交给 Hostower。

  ```bash
  ssh-keygen -t ed25519 -f hostower_key -N ""
  ```

  把 `hostower_key.pub` 的内容写进 hostower 账号的 `authorized_keys`，把 `hostower_key` 的内容填进 Hostower 的凭据。
- **省事：复制 root 的公钥**。

  ```bash
  cp /root/.ssh/authorized_keys /home/hostower/.ssh/
  ```

  代价是同一把私钥既能登 hostower 也能登 root，Hostower 一旦泄露就等于泄露了 root。这样做的话，最好在 `/etc/ssh/sshd_config` 里设置 `PermitRootLogin no`，然后重启 sshd。改之前先确认能用 hostower 登录，并且能 `sudo -i` 切到 root，免得把自己锁在外面。

### 不记得私钥对应的公钥是哪个了

在存着私钥的电脑上导出公钥：

```bash
ssh-keygen -y -f 私钥文件
```

### 报错 `ssh: unable to authenticate, attempted methods [none publickey]`

这说明网络已经连上了，但主机不认这把私钥。按常见程度依次检查：

1. **公钥没放进这个账号的 `authorized_keys`**。用 `sudo cat /home/hostower/.ssh/authorized_keys` 看看；如果提示文件不存在，就是还没放。
2. **权限太宽**：sshd 会直接忽略这种公钥。`.ssh` 目录要是 700，`authorized_keys` 要是 600，属主都要是这个账号。
3. **用户名不对**：凭据里填的用户名，要和公钥所在的账号一致。
4. **私钥粘贴不完整**：缺了开头或结尾那一行，或者换行被吞掉了。

用你自己的电脑验证一下：

```bash
ssh -i 私钥文件 -o IdentitiesOnly=yes hostower@192.0.2.10
```

如果这里也登不上，看主机上的 sshd 日志：

```bash
sudo journalctl -u ssh -u sshd -n 30 --no-pager
```

日志里出现 `bad ownership or modes` 是权限问题；出现 `Invalid user` 是用户名不对。如果你自己能登上而 Hostower 登不上，就到“凭据”里重新粘贴一次私钥。

配好之后不用做别的，Hostower 会自动重连。

### 报错“主机的公钥与首次连接时不一致”

Hostower 第一次连上主机时会记下它的公钥，以后不一致就拒绝连接。这种情况可能是有人在中间冒充，也可能是主机重装了系统。确认是重装之后，在运行 Hostower 的机器上清除旧记录：

```bash
hostower hostkey forget web-1
```

---

## sudo

### 要不要给账号配 sudo？怎么配？

装软件包、装 Docker、改防火墙这些操作都需要 root。不是 root 的账号会用 `sudo -n`（免密，不会卡着等你输密码）。有两种配法：

**方案一：免密执行全部命令（最省事）**

```bash
echo 'hostower ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/hostower
chmod 440 /etc/sudoers.d/hostower
visudo -c
```

`visudo -c` 一定要跑，显示 `parsed OK` 才说明语法没错。sudoers 写坏了，sudo 可能整个都用不了。

这样配之后，这个账号在权限上接近 root。保护来自 Hostower 自己：每个变更都要判级、要你批准，并且记进审计。和直接用 root 登录相比，仍然保留了两点好处：root 可以禁止 SSH 登录；日志能分清哪些操作来自 Hostower。

**方案二：只放行部分命令（白名单）**

适合只让 Hostower 做少数几件事的场景，比如：

```
hostower ALL=(root) NOPASSWD: /usr/bin/systemctl, /usr/bin/journalctl
```

要注意的是，`env`、`tee`、`docker`、`sh` 这类命令一旦放进白名单，就能借它们执行任意命令或写任意文件，效果和方案一一样。而“安装 Docker”这类功能恰好要用到它们，所以只能配合方案一使用。

**检查是否生效**：

```bash
su - hostower -c 'sudo -n true && echo ok'
```

### 老系统上报 `you must have a tty`

CentOS 7 这类老系统的 sudoers 里可能有 `Defaults requiretty`，需要给这个账号单独关掉：

```bash
echo 'Defaults:hostower !requiretty' >> /etc/sudoers.d/hostower
visudo -c
```

---

## Docker 和容器应用

### 安装应用时报“这台主机没有 Docker Compose v2”

主机上没有装 Docker，或者 Docker 有了但缺 `docker compose` 插件。现在安装应用的对话框和容器页都会先检查：缺的话会提示，并给出 **［安装 Docker］** 按钮。

- 点按钮会生成一个请求（L1）。审批时能看到为这台主机拼好的每一步命令，批准后才会执行。
- Debian、Ubuntu、Raspbian、CentOS 系（Rocky、AlmaLinux、RHEL）和 Fedora 从 Docker 官方软件源安装；Alpine、Arch、openSUSE 用系统自带的软件源。不会用“下载脚本直接执行”的方式（`curl … | sh`）。
- 已经有 Docker、只缺 Compose 时，只装 Compose 插件。
- 需要账号是 root，或者能免密 sudo（见上一节），否则不会出现按钮。
- 系统认不出来时，请按 Docker 官方文档手动安装。

### 装好 Docker 后容器页提示 `permission denied … docker.sock`

登录账号不在 `docker` 组里。Hostower 不会替自己加这个组：加进 docker 组就等于拿到了 root 权限，这一步只能由人在主机上做。

1. 在主机上用 root 执行（容器页上可以直接复制这条命令）：

   ```bash
   usermod -aG docker hostower
   ```

2. 回到容器页，点 **［重新连接］**。

账号加进新的组之后，要重新登录才会生效。“重新连接”会断开 Hostower 到这台主机的连接，再重新登录一次，不用重启 Hostower。注意：断开时，这台主机上正在执行的命令会被中断。

宿主机模式下的 `local` 主机是例外：它用的是 Hostower 进程自己的身份，必须重启 Hostower 才能生效。

### 为什么容器名是 `3x-ui-3x-ui-1` 这种重复的名字？

这是 Docker Compose 默认的命名（项目名-服务名-序号），旧版本会出现这种情况。现在：只有一个服务的应用，容器名就是实例名；有多个服务的应用，容器名是“实例名-服务名”。

已经装好的应用，在“已安装”里改一次设置或更新一次，容器就会换成新名字，数据目录不受影响。如果主机上已经有一个同名、但不是 Hostower 装的容器，部署时会报名字冲突，需要先处理掉它。

### 固定版本标签的镜像（比如 `v3.9.0`）怎么更新？

容器页会读取镜像仓库里的标签列表：

- 有同一个大版本里的新版本时，显示“有新版本 vX”，点更新会换成新标签，按原来的设置重建容器。
- 跨大版本的新版本只提示，不自动换，因为大版本升级可能有不兼容的改动，请先看一下更新说明。

---

## 增强采集

### 装好增强采集后，页面多久才会更新？

批准并执行成功后，Hostower 会马上重连这台主机的采样，一般几秒内就能看到“增强采集 版本号”和进程排行。等待期间按钮会依次显示“等待批准”“正在安装…”“正在切换采集方式…”，避免重复提交。卸载也一样。

---

## 更新 Hostower

### 更新后页面上看不到新功能

Hostower 是单页应用。服务器更新重启后，已经打开的页面会自动重连，“设置”里显示的版本号也会变成新的，但页面本身的代码还是旧的。按 `Ctrl+F5`（Mac 上按 `Cmd+Shift+R`）强制刷新一次就好。

### 为什么“设置”里显示“这个构建没有配置更新来源”？

自动更新只对正式发布的版本有效：它要用发布公钥验证签名。自己编译的版本没有发布公钥，需要自己替换二进制或镜像来更新。

### 初始密码在哪？

在数据目录的 `initial-password` 文件里，不会写进日志。用 Docker 部署时：

```bash
sudo cat data/initial-password   # 在 compose.yaml 所在的目录执行
```

登录后请尽快修改密码，并添加 Passkey 或动态码。所有登录方式都丢了的话，在服务器上运行 `hostower auth reset-password`。
