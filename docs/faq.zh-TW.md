# 常見問題

[简体中文](faq.md) | **繁體中文** | [English](faq.en.md) | [日本語](faq.ja.md) | [한국어](faq.ko.md) | [Français](faq.fr.md) | [Deutsch](faq.de.md)

接入主機、安裝 Docker、安裝應用這幾步裡最常遇到的問題和注意事項。範例裡的主機名稱、位址都是預留位置，請換成你自己的。

- [接入主機](#接入主機)
- [sudo](#sudo)
- [Docker 與容器應用](#docker-與容器應用)
- [站點、憑證與 Fail2ban](#站點憑證與-fail2ban)
- [增強採集](#增強採集)
- [流量配額](#流量配額)
- [更新 Hostower](#更新-hostower)

---

## 接入主機

### 為什麼 SSH 憑據不能用 root？

Hostower 等於「所有主機的鑰匙」，所以在主機這邊還要有一道它自己管不到的門：

1. **面板失守時控制損失**：資料庫和主金鑰一旦外洩，攻擊者能拿到 Hostower 存的私鑰。root 的私鑰意味著直接接管整台主機；受限帳號只能做 sudo 允許的事。
2. **判級不可能完美**：判級引擎要剖析 shell，總可能有沒想到的繞過寫法。sudo 的限制由主機系統執行，是獨立的第二道保險。
3. **互動終端機不逐條判級**：開啟終端機要按 L2 核准、全程錄影，但裡面輸入的命令不再逐條判級。用 root 登入，一旦核准就什麼都能做。
4. **維運習慣**：很多系統預設禁止 root 用 SSH 登入；用專用帳號登入，主機的 `auth.log` 和 sudo 日誌裡能分清哪些是 Hostower 做的。

這是硬性限制：使用者名稱填 `root` 會直接被拒絕。

### 怎麼為 Hostower 建立一個專用帳號？

在受管主機上用 root 執行：

```bash
useradd -m -s /bin/bash hostower
mkdir -p /home/hostower/.ssh
echo 'ssh-ed25519 AAAA…（公鑰）' >> /home/hostower/.ssh/authorized_keys
chown -R hostower:hostower /home/hostower/.ssh
chmod 700 /home/hostower/.ssh
chmod 600 /home/hostower/.ssh/authorized_keys
```

然後按下一節設定 sudo，再在 Hostower 的「憑據」裡新增 SSH 憑據：使用者名稱填 `hostower`，私鑰填和上面公鑰配對的那一把（從 `-----BEGIN` 到 `-----END` 整段）。

### root 已經能用金鑰登入了，新帳號還要再設定金鑰嗎？

要。公鑰是按帳號放的：每個帳號只認自己家目錄裡 `~/.ssh/authorized_keys` 的公鑰，`/root/.ssh/authorized_keys` 只對 root 有效。

兩種做法：

- **建議：為 Hostower 單獨產生一把金鑰**。root 繼續用你原來的金鑰，那把私鑰只留在你自己手裡，不交給 Hostower。

  ```bash
  ssh-keygen -t ed25519 -f hostower_key -N ""
  ```

  把 `hostower_key.pub` 的內容寫進 hostower 帳號的 `authorized_keys`，把 `hostower_key` 的內容填進 Hostower 的憑據。
- **省事：複製 root 的公鑰**。

  ```bash
  cp /root/.ssh/authorized_keys /home/hostower/.ssh/
  ```

  代價是同一把私鑰既能登入 hostower 也能登入 root，Hostower 一旦外洩就等於外洩了 root。這樣做的話，最好在 `/etc/ssh/sshd_config` 裡設定 `PermitRootLogin no`，然後重新啟動 sshd。修改之前先確認能用 hostower 登入，並且能 `sudo -i` 切換到 root，以免把自己鎖在外面。

### 不記得私鑰對應的公鑰是哪個了

在存著私鑰的電腦上匯出公鑰：

```bash
ssh-keygen -y -f 私鑰檔案
```

### 錯誤訊息 `ssh: unable to authenticate, attempted methods [none publickey]`

這表示網路已經連上了，但主機不認這把私鑰。按常見程度依序檢查：

1. **公鑰沒放進這個帳號的 `authorized_keys`**。用 `sudo cat /home/hostower/.ssh/authorized_keys` 看看；如果提示檔案不存在，就是還沒放。
2. **權限太寬**：sshd 會直接忽略這種公鑰。`.ssh` 目錄要是 700，`authorized_keys` 要是 600，擁有者都要是這個帳號。
3. **使用者名稱不對**：憑據裡填的使用者名稱，要和公鑰所在的帳號一致。
4. **私鑰貼上不完整**：缺了開頭或結尾那一行，或者換行被吃掉了。

用你自己的電腦驗證一下：

```bash
ssh -i 私鑰檔案 -o IdentitiesOnly=yes hostower@192.0.2.10
```

如果這裡也登入不了，查看主機上的 sshd 日誌：

```bash
sudo journalctl -u ssh -u sshd -n 30 --no-pager
```

日誌裡出現 `bad ownership or modes` 是權限問題；出現 `Invalid user` 是使用者名稱不對。如果你自己能登入而 Hostower 登入不了，就到「憑據」裡重新貼上一次私鑰。

設定好之後不用做別的，Hostower 會自動重新連線。

### 錯誤訊息「主機的公鑰與首次連線時不一致」

Hostower 第一次連上主機時會記下它的公鑰，以後不一致就拒絕連線。這種情況可能是有人在中間冒充，也可能是主機重新安裝了系統。確認是重新安裝之後，在執行 Hostower 的機器上清除舊紀錄：

```bash
hostower hostkey forget web-1
```

---

## sudo

### 要不要為帳號設定 sudo？怎麼設定？

安裝套件、安裝 Docker、修改防火牆這些操作都需要 root。不是 root 的帳號會用 `sudo -n`（免密碼，不會卡著等你輸入密碼）。有兩種設定方式：

**方案一：免密碼執行全部命令（最省事）**

```bash
echo 'hostower ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/hostower
chmod 440 /etc/sudoers.d/hostower
visudo -c
```

`visudo -c` 一定要執行，顯示 `parsed OK` 才代表語法沒錯。sudoers 寫壞了，sudo 可能整個都無法使用。

這樣設定之後，這個帳號在權限上接近 root。保護來自 Hostower 自己：每個變更都要判級、要你核准，並且記入稽核。和直接用 root 登入相比，仍然保留了兩點好處：root 可以禁止 SSH 登入；日誌能分清哪些操作來自 Hostower。

**方案二：只放行部分命令（白名單）**

適合只讓 Hostower 做少數幾件事的情境，例如：

```
hostower ALL=(root) NOPASSWD: /usr/bin/systemctl, /usr/bin/journalctl
```

要注意的是，`env`、`tee`、`docker`、`sh` 這類命令一旦放進白名單，就能藉由它們執行任意命令或寫入任意檔案，效果和方案一一樣。而「安裝 Docker」這類功能恰好要用到它們，所以只能搭配方案一使用。

**檢查是否生效**：

```bash
su - hostower -c 'sudo -n true && echo ok'
```

### 舊系統上出現 `you must have a tty`

CentOS 7 這類舊系統的 sudoers 裡可能有 `Defaults requiretty`，需要為這個帳號單獨關掉：

```bash
echo 'Defaults:hostower !requiretty' >> /etc/sudoers.d/hostower
visudo -c
```

---

## Docker 與容器應用

### 安裝應用時出現「這台主機沒有 Docker Compose v2」

主機上沒有安裝 Docker，或者 Docker 有了但缺少 `docker compose` 外掛程式。現在安裝應用的對話框和容器頁都會先檢查：缺少的話會提示，並提供 **［安裝 Docker］** 按鈕。

- 點按鈕會產生一個請求（L1）。審核時能看到為這台主機組好的每一步命令，核准後才會執行。
- Debian、Ubuntu、Raspbian、CentOS 系（Rocky、AlmaLinux、RHEL）和 Fedora 從 Docker 官方套件庫安裝；Alpine、Arch、openSUSE 用系統內建的套件庫。不會用「下載指令碼直接執行」的方式（`curl … | sh`）。
- 已經有 Docker、只缺 Compose 時，只安裝 Compose 外掛程式。
- 需要帳號是 root，或者能免密碼 sudo（見上一節），否則不會出現按鈕。
- 系統辨識不出來時，請按 Docker 官方文件手動安裝。

### 裝好 Docker 後容器頁提示 `permission denied … docker.sock`

登入帳號不在 `docker` 群組裡。Hostower 不會替自己加入這個群組：加入 docker 群組就等於拿到了 root 權限，這一步只能由人在主機上操作。

1. 在主機上用 root 執行（容器頁上可以直接複製這條命令）：

   ```bash
   usermod -aG docker hostower
   ```

2. 回到容器頁，點 **［重新連線］**。

帳號加入新的群組之後，要重新登入才會生效。「重新連線」會中斷 Hostower 到這台主機的連線，再重新登入一次，不用重新啟動 Hostower。注意：中斷時，這台主機上正在執行的命令會被中斷。

宿主機模式下的 `local` 主機是例外：它用的是 Hostower 處理程序自己的身分，必須重新啟動 Hostower 才能生效。

### 為什麼容器名稱是 `3x-ui-3x-ui-1` 這種重複的名稱？

這是 Docker Compose 預設的命名方式（專案名稱-服務名稱-序號），舊版本會出現這種情況。現在：只有一個服務的應用，容器名稱就是實例名稱；有多個服務的應用，容器名稱是「實例名稱-服務名稱」。

已經安裝好的應用，在「已安裝」裡修改一次設定或更新一次，容器就會換成新名稱，資料目錄不受影響。如果主機上已經有一個同名、但不是 Hostower 安裝的容器，部署時會回報名稱衝突，需要先處理掉它。

### 固定版本標籤的映像檔（例如 `v3.9.0`）怎麼更新？

容器頁會讀取映像檔倉庫裡的標籤清單：

- 有同一個主要版本裡的新版本時，顯示「有新版本 vX」，點更新會換成新標籤，按原來的設定重建容器。
- 跨主要版本的新版本只提示，不自動更換，因為主要版本升級可能有不相容的改動，請先看一下更新說明。

---

## 站點、憑證與 Fail2ban

### Caddy 和 Nginx 選哪個？

在主機詳情的「網路」頁裡，兩個都能一鍵安裝（用系統軟體源），都能建反向代理、靜態目錄和跳轉三種站點。

- **Caddy**：自動申請和續期 HTTPS 憑證，最省事，沒有特別的理由就選它。
- **Nginx**：熟悉 Nginx，或者主機上已經有 Nginx 設定時選它。憑證可以用 Let's Encrypt 自動申請（Hostower 會順帶裝上 certbot，它自己定時續期），也可以填主機上已有憑證的路徑。

兩個都要用 80 和 443 連接埠，Hostower 每台主機只裝一個。它只管自己寫的檔案（`/etc/caddy/sites.d/`、`/etc/nginx/conf.d/hostower-*.conf`），主機上原有的設定不動。

### Let's Encrypt 憑證申請失敗

申請失敗時，站點先以 HTTP 提供，不會影響別的站點。常見原因：

- 網域還沒有解析到這台主機（或者解析還沒生效）。
- 80 連接埠從外面存取不到：主機的防火牆、雲端服務商的安全群組都要放行 80 和 443。
- 80 連接埠被別的程式佔了。
- 同一個網域短時間裡申請太多次，觸發了 Let's Encrypt 的限額（同一組網域每週最多 5 張）。

排除之後，撤下這個站點再重新新增。

### 443 連接埠已經被 3x-ui（Xray）佔用了

兩個程式不能同時監聽同一個連接埠。可以把 Xray 的入站改到別的連接埠；或者給站點寫帶連接埠的位址（比如 `app.example.com:8443`），這時 Nginx 只能用已有的憑證，Caddy 仍然可以自動申請。

### Fail2ban 會不會把我或 Hostower 封掉？

白名單裡固定有本機迴環位址和 Hostower 連這台主機用的來源位址，Hostower 用金鑰登入，也不會留下失敗紀錄。你自己的出口 IP 不固定時，把常用的網段加進白名單。

如果還是被封了：在「網路」頁被封的 IP 旁點解封；或者從別的位址、雲端服務商的主控台登入主機執行：

```bash
sudo fail2ban-client set sshd unbanip 203.0.113.7
```

Fail2ban 改的是防火牆規則，所以安裝、改設定、解封都是高危操作（L2），只能在網頁上批准，批准時要重新驗證身分。

---

## 增強採集

### 裝好增強採集後，頁面多久才會更新？

核准並執行成功後，Hostower 會馬上重新連線這台主機的取樣，一般幾秒內就能看到「增強採集 版本號」和處理程序排行。等待期間按鈕會依序顯示「等待核准」「正在安裝…」「正在切換採集方式…」，避免重複提交。解除安裝也一樣。

---

## 流量配額

### 面板上的月流量和服務商面板上的對不上

主機頁「流量」卡片和總覽「流量排行」裡的數字來自主機各網路卡的累計計數器，按下面的口徑算：

- **只算出口網路卡**。WireGuard、OpenVPN、Tailscale 這類通道，`br0`、`vmbr0` 這類橋接，`eth0.100` 這類 VLAN 子介面，以及 bond 的成員網路卡上走的是同一份流量，不重複算；`lo`、`docker0`、`veth…` 這類虛擬網路卡也不算。只想按某一塊網路卡計費時，在「設定配額」裡選「統計的網路卡」。
- **日期按面板所在機器的時區劃分**。服務商按別的時區結帳時，月初、月底那一天會差幾個小時的流量。
- **面板斷線或重新啟動期間的流量會補上**；但主機重新開機時，從最後一次採樣到重新開機之間那幾秒的流量會漏掉。
- **接入面板之前用掉的流量測不到**。在「設定配額」裡填上服務商面板顯示的本週期已用量，本週期就按它對齊；校準只對目前週期有效，下個週期自動失效。改了計費方式、網路卡或重置日，原來的校準也會清掉。

一切以服務商的帳單為準。

### 升級後網路速率比以前低了

網路速率現在和月流量用同一個口徑，只算出口網路卡。以前通道、橋接上的流量會和實體網路卡上的疊加，同一份流量被算了兩遍，所以數字偏高。之前已經存下的歷史點仍是舊口徑。

### 流量告警沒有收到

到達告警百分比（預設 80%、95%、100%）和「依目前速度將超出額度」時，通知發到訂閱了「流量到達告警線」「預計超出流量額度」的通知管道；每個週期每一檔只發一次。升級時已有的通知管道會自動勾上這兩項；如果你之前取消過，到「設定 → 通知與機器人」裡重新勾選。「預計超出」要等週期裡有滿一天的資料才會判斷。

---

## 更新 Hostower

### 更新後頁面上看不到新功能

Hostower 是單頁應用程式。伺服器更新並重新啟動後，已經開啟的頁面會自動重新連線，「設定」裡顯示的版本號也會變成新的，但頁面本身的程式碼還是舊的。按 `Ctrl+F5`（Mac 上按 `Cmd+Shift+R`）強制重新整理一次就好。

### 為什麼「設定」裡顯示「這個建置沒有設定更新來源」？

自動更新只對正式發行的版本有效：它要用發行公鑰驗證簽章。自行編譯的版本沒有發行公鑰，需要自己替換二進位檔或映像檔來更新。

### 初始密碼在哪裡？

在資料目錄的 `initial-password` 檔案裡，不會寫進日誌。使用 Docker 部署時：

```bash
sudo cat data/initial-password   # 在 compose.yaml 所在的目錄執行
```

登入後請盡快修改密碼，並新增 Passkey 或動態驗證碼。所有登入方式都遺失的話，在伺服器上執行 `hostower auth reset-password`。
