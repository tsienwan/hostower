# FAQ

[简体中文](faq.md) | [繁體中文](faq.zh-TW.md) | **English** | [日本語](faq.ja.md) | [한국어](faq.ko.md) | [Français](faq.fr.md) | [Deutsch](faq.de.md)

The most common problems and tips when connecting hosts, installing Docker and installing apps. Host names and addresses in the examples are placeholders; replace them with your own.

- [Connecting hosts](#connecting-hosts)
- [sudo](#sudo)
- [Docker and container apps](#docker-and-container-apps)
- [Sites, certificates and Fail2ban](#sites-certificates-and-fail2ban)
- [Enhanced collector](#enhanced-collector)
- [Traffic quotas](#traffic-quotas)
- [Updating Hostower](#updating-hostower)

---

## Connecting hosts

### Why can't an SSH credential use root?

Hostower holds "the keys to every host", so each host needs one more gate that Hostower itself cannot open:

1. **Limit the damage if the panel is compromised**: if the database and master key leak, an attacker gets the private keys Hostower stores. A root key means full control of the host; a restricted account can only do what sudo allows.
2. **Classification can never be perfect**: the risk engine has to parse shell, and there may always be a bypass nobody thought of. sudo restrictions are enforced by the host's own system — an independent second line of defense.
3. **The interactive terminal is not classified command by command**: opening a terminal needs an L2 approval and is fully recorded, but the commands typed inside are not classified one by one. Logged in as root, anything goes once it is approved.
4. **Operational practice**: many systems forbid root SSH logins by default; with a dedicated account, the host's `auth.log` and sudo log show exactly what Hostower did.

This is a hard rule: a user name of `root` is rejected outright.

### How do I create a dedicated account for Hostower?

On the managed host, as root:

```bash
useradd -m -s /bin/bash hostower
mkdir -p /home/hostower/.ssh
echo 'ssh-ed25519 AAAA… (public key)' >> /home/hostower/.ssh/authorized_keys
chown -R hostower:hostower /home/hostower/.ssh
chmod 700 /home/hostower/.ssh
chmod 600 /home/hostower/.ssh/authorized_keys
```

Then set up sudo (next section), and add an SSH credential under **Credentials** in Hostower: user name `hostower`, and the private key that matches the public key above (the whole block from `-----BEGIN` to `-----END`).

### root can already log in with a key. Does the new account need its own key setup?

Yes. Public keys are per account: each account only trusts the keys in its own `~/.ssh/authorized_keys`, and `/root/.ssh/authorized_keys` applies to root only.

Two ways to do it:

- **Recommended: generate a separate key for Hostower.** Keep using your original key for root; that private key stays with you and is never given to Hostower.

  ```bash
  ssh-keygen -t ed25519 -f hostower_key -N ""
  ```

  Put the contents of `hostower_key.pub` into the hostower account's `authorized_keys`, and the contents of `hostower_key` into the Hostower credential.
- **Quick: copy root's public keys.**

  ```bash
  cp /root/.ssh/authorized_keys /home/hostower/.ssh/
  ```

  The cost: the same private key can log in as both hostower and root, so a leak from Hostower is a root leak. If you do this, set `PermitRootLogin no` in `/etc/ssh/sshd_config` and restart sshd. Before you do, make sure you can log in as hostower and `sudo -i` to root, so you don't lock yourself out.

### I don't remember which public key goes with my private key

Derive it on the computer that has the private key:

```bash
ssh-keygen -y -f private_key_file
```

### Error: `ssh: unable to authenticate, attempted methods [none publickey]`

The network connection works, but the host does not accept this private key. Check, from most to least common:

1. **The public key is not in this account's `authorized_keys`.** Check with `sudo cat /home/hostower/.ssh/authorized_keys`; "No such file or directory" means it was never added.
2. **Permissions are too open**: sshd silently ignores such keys. `.ssh` must be 700 and `authorized_keys` 600, both owned by the account.
3. **Wrong user name**: the user name in the credential must be the account that holds the public key.
4. **The private key was pasted incompletely**: the first or last line is missing, or line breaks were lost.

Test from your own computer:

```bash
ssh -i private_key_file -o IdentitiesOnly=yes hostower@192.0.2.10
```

If that fails too, read the sshd log on the host:

```bash
sudo journalctl -u ssh -u sshd -n 30 --no-pager
```

`bad ownership or modes` means a permission problem; `Invalid user` means a wrong user name. If you can log in but Hostower can't, paste the private key into **Credentials** again.

Once fixed, there is nothing else to do: Hostower reconnects automatically.

### Error: the host key does not match the first connection

Hostower records a host's key on first connection and refuses to connect if it changes later. Someone may be intercepting the connection, or the host may have been reinstalled. Once you have confirmed a reinstall, clear the old record on the machine that runs Hostower:

```bash
hostower hostkey forget web-1
```

---

## sudo

### Should the account have sudo? How do I set it up?

Installing packages, installing Docker and changing the firewall all need root. A non-root account uses `sudo -n` (passwordless; it never waits for a password). Two options:

**Option 1: passwordless sudo for everything (simplest)**

```bash
echo 'hostower ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/hostower
chmod 440 /etc/sudoers.d/hostower
visudo -c
```

Always run `visudo -c` and look for `parsed OK`: a broken sudoers file can disable sudo entirely.

With this, the account is close to root in what it can do. The protection comes from Hostower itself: every change is classified, approved by you and audited. Compared with logging in as root directly, two benefits remain: root SSH logins can be disabled, and the logs show which actions came from Hostower.

**Option 2: allow only some commands (allow list)**

Suitable when Hostower should only do a few things, for example:

```
hostower ALL=(root) NOPASSWD: /usr/bin/systemctl, /usr/bin/journalctl
```

Note that commands such as `env`, `tee`, `docker` and `sh` let you run anything or write any file, so putting them on the list is equivalent to option 1. Features like **Install Docker** need exactly those commands, so they only work with option 1.

**Check that it works:**

```bash
su - hostower -c 'sudo -n true && echo ok'
```

### Older systems report `you must have a tty`

On older systems such as CentOS 7, sudoers may contain `Defaults requiretty`. Turn it off for this account:

```bash
echo 'Defaults:hostower !requiretty' >> /etc/sudoers.d/hostower
visudo -c
```

---

## Docker and container apps

### Installing an app fails with "this host has no Docker Compose v2"

The host has no Docker, or it has Docker without the `docker compose` plugin. The app install dialog and the Containers page now check first and, if something is missing, show a note with an **Install Docker** button.

- The button creates a request (L1). When approving, you see every command assembled for this specific host; nothing runs until you approve.
- Debian, Ubuntu, Raspbian, the CentOS family (Rocky, AlmaLinux, RHEL) and Fedora install from Docker's official repositories; Alpine, Arch and openSUSE use their own repositories. It never downloads and runs a script (`curl … | sh`).
- If Docker is present and only Compose is missing, only the Compose plugin is installed.
- The account must be root or have passwordless sudo (see the previous section); otherwise there is no button.
- If the system is not recognized, install Docker manually following Docker's official documentation.

### After installing Docker, the Containers page reports `permission denied … docker.sock`

The login account is not in the `docker` group. Hostower does not add itself to that group: membership is equivalent to root, so only a person on the host should grant it.

1. On the host, as root (you can copy the command from the Containers page):

   ```bash
   usermod -aG docker hostower
   ```

2. Back on the Containers page, click **Reconnect**.

A new group only takes effect after logging in again. **Reconnect** closes Hostower's connection to this host and logs in once more, without restarting Hostower. Note: commands running on this host at that moment are interrupted.

The `local` host in host mode is an exception: it runs as the Hostower process itself, so you must restart Hostower.

### Why are containers named like `3x-ui-3x-ui-1`?

That is Docker Compose's default naming (project-service-index), which older versions produced. Now an app with a single service names its container after the instance; an app with several services uses "instance-service".

For apps already installed, edit their settings or update them once under **Installed** and the container is recreated with the new name; data directories are untouched. If the host already has a container with the same name that Hostower did not create, deployment reports a name conflict; deal with that container first.

### An app is installed. Where do I open it, and what are the initial account and password?

Under **Apps → Installed**, click the rocket icon (**Getting started**) on that row: it shows the address (you can click it to open), notes on the initial account and the next few steps. Once an install request has run successfully, the request's detail page has the same entry point.

- For apps whose initial password can be set at install time (OpenList's admin password, LiteLLM's master key, database passwords), the password is the app secret you chose during installation.
- Apps where you create the administrator yourself on first visit (Uptime Kuma, Portainer, Open WebUI, New API, n8n) have no initial account; open them soon after installing and complete the registration.
- 3x-ui's initial user name and password are generated randomly on first start and appear only once, in the container log: in the getting-started guide click **Read the initial user name and password**, and the panel reads that line with a single read-only query. After the container has been recreated the log no longer contains it; in that case reset them in a terminal on the host with the command given in the guide.

If the address does not open, first check whether the listen address is `127.0.0.1` (reachable only from the host itself; set up a reverse proxy, or change it to `0.0.0.0` under **Edit settings**), then check whether the host firewall and the cloud provider's security group allow that port.

### **Open panel** opens the wrong address, or the app listens only on `127.0.0.1`

Apps with a web panel (OpenList, 3x-ui, Uptime Kuma, Portainer, LiteLLM, Open WebUI, New API, n8n) have an **Open panel** button on their row under **Installed**. By default the address is built from the instance's current listen address and port, so after you change them under **Edit settings** the button follows along. When that isn't right, click the link icon next to it to set a **Panel address** (only the owner can change it; it is stored only in Hostower, changes nothing on the host, and every change is audited):

- **Directly on the host**: the port or path was changed inside the app itself (for example, the port and panel path under 3x-ui's "Panel Settings"), which Hostower can't see. Enter the new port and path here.
- **Full address**: the app sits behind a reverse proxy, is reached by a domain, or listens only on `127.0.0.1`. Enter the address the browser should open, such as `https://panel.example.com/xui/`. Sites on the Network page that already proxy to this port are listed and can be filled in with one click.
- **Through an SSH tunnel**: if you don't want to open the port to the outside, the page gives you an `ssh -N -L local-port:127.0.0.1:app-port user@host` command. Run it on your own computer, and the button opens `http://localhost:local-port/path`.

The panel address can't contain a user name, password or anything after `?` or `#`, so no credentials ever end up in the button's link.

### What do “Restricted” and “Unreachable” mean on the “Environment” page?

**Environment** on the host page and **Apps → Environment** send a batch of read-only requests from this host and look at how each service answers:

- **AI services' region and risk controls**: which region ChatGPT, Claude and Google (using the region YouTube assigns) place this host in, and whether that region is within the usual service coverage; whether the ChatGPT website demands human verification and whether Google Search flags the address as unusual traffic; and whether the IP database's location matches the region the AI services see (a mismatch often gets the host treated as being in an unsupported region).
- **Model APIs** (OpenAI, Anthropic, Google Gemini, xAI, DeepSeek): the request is sent without a key. An answer of “missing key” means **Available**; add a key and you can call it. A refusal based on the country or region of the egress means **Restricted**; use a host in another region, or route through a gateway such as LiteLLM or New API.
- **Websites and streaming** (Google, YouTube, Netflix, Disney+, TikTok, Facebook, X, the ChatGPT web app and others): **Available** if the site opens normally; **Restricted** if it refuses or redirects to a “not available in your region” page. For YouTube and Netflix the region you land in is shown as well; Netflix is judged with a non-original title, and “Originals only” means that region's catalog is not open to this address.
- **Unreachable**: no connection within 8 seconds; the network may be blocking it, or the host may not have `curl`.
- **Uncertain**: the connection worked, but the answer is none of the above, so no conclusion can be drawn.

Only the way each service responds is checked here; no account is signed in. Each provider's web app and subscription sign-in have their own region and risk-control rules, so neither an available API nor a site that opens guarantees that an account will work. The same page also lists which AI CLI tools are installed on the host (Claude Code, Codex CLI, Gemini CLI and others) and the runtimes; versions are read from the installation directories, and these programs are never executed.

### What are the type and risk score under “Egress IP”? How accurate are they?

Websites see the host's egress address. What type it is and whether it has a bad record affect whether you get restricted and how often you have to pass human verification:

- **Type**: data center, residential broadband, business line, mobile network and so on. A VPS almost always has a data center address; streaming services often show it originals only, and subscription sign-ins for AI services are more likely to trip risk control.
- **Proxy flag**: whether the database has marked it as a proxy or VPN exit.
- **Risk score**: 0 to 100; the higher it is, the more abuse has been recorded for this address or its network range. Below 33 counts as low, above 66 as high.
- Also shown: the operator (ASN), the location, and whether traffic goes through Cloudflare WARP.
- IPv4 and IPv6 are checked and shown separately: on hosts with IPv6, many websites prefer IPv6, and the two egresses may differ in ownership and reputation.

The host gets this information by looking up its own egress address in a public IP database (proxycheck.io). It is for reference only: different databases may reach different conclusions, and websites' risk control does not rely on these alone. When the lookup fails, “Not found” is shown and the other checks are unaffected.

### What is “Agent collaboration”? Will the assistants mess with my hosts?

The “Agent collaboration” page lets you dispatch the same task (for example “check the nginx configuration and recent error logs”) to Claude Code, Codex CLI or Gemini CLI on several hosts; the assistant on each host only works on its own host. When they finish, you can view each host's conclusions side by side, then click “Summarize” to have the assistant on one of the hosts compare and sum them up.

- Dispatching a task and summarizing are both requests: you first see the command that will run on each host and its risk level, nothing runs until it is approved, and the audit log records it as usual.
- Read-only by default: the assistant can look but not change anything, and asking it to run commands is refused. Ticking “Allow changes to files in the working directory” only opens up changes inside the working directory (acceptEdits for Claude Code, the workspace-write sandbox for Codex, auto_edit for Gemini CLI).
- The page has no switch for skipping permission prompts or the sandbox; such commands typed by hand (for example `--dangerously-skip-permissions` or `--yolo`) are rated high-risk (L2).
- The assistant must first be installed and configured (API key or subscription sign-in) from the app catalog. You'll be told which hosts don't have it.

The collaboration map at the top of the page shows live what the tower is sending to which host (colors distinguish Agent tasks, file distribution, app installs and other commands), which assistant is thinking, and which LiteLLM / New API instance each assistant calls models through. When the system's “reduce motion” setting is on, the map's animations stop.

### How do I track how many tokens each AI tool uses?

Install LiteLLM or New API (under **AI tools** in the app catalog), point the API address of Claude Code, Codex CLI and the other tools at it, and issue a separate key for each tool or each person. The number of calls, token usage and cost per key and per model are shown in the gateway's own admin interface. Hostower itself neither proxies nor records model requests.

### How do I send the same file to several hosts? How do I collect logs from several hosts?

Use **File sync** in the left sidebar:

- **Distribute to hosts**: drag the files in (up to 20 at a time), enter the target directory and pick the hosts. Each file becomes one request: you first see where it will be written on each host and the risk level, and nothing is written until it is approved. A directory that does not exist is created automatically, and a file with the same name is overwritten. Writing to system directories requires the login account to have permission; writing to sensitive paths such as `/etc/ssh` is classified as high-risk.
- **Collect from hosts**: enter the directory and a file-name pattern (for example `*.log`; `*` stands for any run of characters and `?` for a single character), pick the hosts and how many levels down to look, click **Search** first to see which files there are, then click **Download archive**. Inside the archive, files are stored as “host name/file path”.

Limits on collecting: at most 200 files per host, no single file larger than 100 MB, and no more than 1 GB in total per download; files that cannot be read (private keys, credential files, or files the login account has no permission for) and those over the limits are listed in “_跳过的文件.txt” (skipped files) inside the archive. Searching is a read-only query; every downloaded file is recorded in the audit log.

### How do images with fixed version tags (such as `v3.9.0`) get updated?

The Containers page reads the tag list from the registry:

- If there is a newer version within the same major version, it shows **New version vX**; updating switches to the new tag and recreates the container with the same settings.
- A newer major version is only mentioned, never applied automatically: major upgrades may contain breaking changes, so read the release notes first.

---

## Sites, certificates and Fail2ban

### Caddy or Nginx?

On the host's “Network” page both can be installed in one click (from the system repositories), and both can serve the three kinds of site: reverse proxy, static directory and redirect.

- **Caddy** obtains and renews HTTPS certificates automatically and is the least work. Pick it unless you have a reason not to.
- **Nginx** suits you if you know Nginx or the host already has Nginx configuration. Certificates can come from Let's Encrypt automatically (Hostower installs certbot along the way, and certbot renews on its own schedule), or you can point to certificates already on the host.

Both need ports 80 and 443, so Hostower installs only one per host. It only manages the files it writes (`/etc/caddy/sites.d/`, `/etc/nginx/conf.d/hostower-*.conf`); existing configuration on the host is left alone.

### Getting a Let's Encrypt certificate failed

When it fails, the site is served over HTTP for now and other sites are unaffected. Common causes:

- The domain does not resolve to this host yet (or the DNS change has not propagated).
- Port 80 is not reachable from outside: both the host firewall and the cloud provider's security group must allow 80 and 443.
- Another program is already using port 80.
- Too many requests for the same domain in a short time hit Let's Encrypt's limits (at most 5 certificates per week for the same set of domains).

Once fixed, remove the site and add it again.

### Port 443 is already used by 3x-ui (Xray)

Two programs cannot listen on the same port. Either move the Xray inbound to another port, or give the site an address with a port (e.g. `app.example.com:8443`); in that case Nginx can only use an existing certificate, while Caddy can still obtain one automatically.

### Can Fail2ban ban me or Hostower?

The allowlist always contains the loopback addresses and the address Hostower connects from, and Hostower logs in with a key, so it leaves no failed attempts. If your own public IP changes, add the networks you usually come from to the allowlist.

If you get banned anyway: click unban next to the IP on the “Network” page, or log in from another address or the cloud provider's console and run:

```bash
sudo fail2ban-client set sshd unbanip 203.0.113.7
```

Fail2ban changes firewall rules, so installing it, changing its settings and unbanning are high-risk operations (L2): they can only be approved on the web, with a fresh identity check.

---

## Enhanced collector

### How long until the page reflects an installed collector?

Once the request is approved and succeeds, Hostower immediately reconnects the host's sampling, so **Enhanced collector &lt;version&gt;** and the process ranking usually appear within seconds. While waiting, the button shows **Awaiting approval**, then **Installing…**, then **Switching collection mode…**, so it can't be submitted twice. Uninstalling works the same way.

---

## Traffic quotas

### Monthly traffic in the panel doesn't match the provider's panel

The figures on the host page's "Traffic" card and in the overview's "Traffic ranking" come from each interface's cumulative counters, counted like this:

- **Only uplink interfaces count.** Tunnels such as WireGuard, OpenVPN and Tailscale, bridges such as `br0` and `vmbr0`, VLAN sub-interfaces such as `eth0.100`, and bond members carry the same traffic and are not counted twice; virtual interfaces such as `lo`, `docker0` and `veth…` are excluded. To bill by a single interface, choose it under "Interface counted" in "Set quota".
- **Days follow the time zone of the machine running the panel.** If your provider bills in another time zone, the first and last day of a period can differ by a few hours of traffic.
- **Traffic while the panel was disconnected or restarting is caught up**; when a host reboots, the few seconds between the last sample and the reboot are lost.
- **Traffic used before the host was connected can't be measured.** Enter the usage your provider shows for this period in "Set quota" and the period is aligned to it. Calibration only applies to the current period and expires with it; changing the billing mode, interface or reset day also clears it.

Your provider's bill is authoritative.

### Network rates are lower after upgrading

Network rates now use the same rule as monthly traffic and only count uplink interfaces. Before, traffic on tunnels and bridges was added on top of the physical interface, so the same traffic was counted twice and the numbers were too high. History stored before the upgrade still uses the old rule.

### I didn't get a traffic alert

When an alert percentage (80%, 95% and 100% by default) is reached, or the quota is projected to be exceeded at the current rate, a notification goes to channels subscribed to "Traffic alert level reached" and "Projected to exceed traffic quota"; each level is sent once per period. Existing channels get both ticked on upgrade; if you unticked them, tick them again under "Settings → Notifications & bot". The projection needs at least a full day of data in the period.

---

## Updating Hostower

### New features don't show up after an update

Hostower is a single-page app. After the server restarts with a new version, open pages reconnect automatically and **Settings** shows the new version number, but the page code itself is still the old one. Hard-refresh once with `Ctrl+F5` (`Cmd+Shift+R` on a Mac).

### Why does Settings say "This build has no update source configured"?

Automatic updates only work for official releases, because they verify the release signature with the release public key. A self-built binary has no release key; update it by replacing the binary or image yourself.

### Where is the initial password?

In the `initial-password` file in the data directory; it is never written to logs. With Docker:

```bash
sudo cat data/initial-password   # run in the directory that holds compose.yaml
```

After signing in, change the password and add a Passkey or authenticator as soon as possible. If every sign-in method is lost, run `hostower auth reset-password` on the server.
