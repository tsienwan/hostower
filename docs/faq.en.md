# FAQ

[简体中文](faq.md) | [繁體中文](faq.zh-TW.md) | **English** | [日本語](faq.ja.md) | [한국어](faq.ko.md) | [Français](faq.fr.md) | [Deutsch](faq.de.md)

The most common problems and tips when connecting hosts, installing Docker and installing apps. Host names and addresses in the examples are placeholders; replace them with your own.

- [Connecting hosts](#connecting-hosts)
- [sudo](#sudo)
- [Docker and container apps](#docker-and-container-apps)
- [Enhanced collector](#enhanced-collector)
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

### How do images with fixed version tags (such as `v3.9.0`) get updated?

The Containers page reads the tag list from the registry:

- If there is a newer version within the same major version, it shows **New version vX**; updating switches to the new tag and recreates the container with the same settings.
- A newer major version is only mentioned, never applied automatically: major upgrades may contain breaking changes, so read the release notes first.

---

## Enhanced collector

### How long until the page reflects an installed collector?

Once the request is approved and succeeds, Hostower immediately reconnects the host's sampling, so **Enhanced collector &lt;version&gt;** and the process ranking usually appear within seconds. While waiting, the button shows **Awaiting approval**, then **Installing…**, then **Switching collection mode…**, so it can't be submitted twice. Uninstalling works the same way.

---

## Updating Hostower

### New features don't show up after an update

Hostower is a single-page app. After the server restarts with a new version, open pages reconnect automatically and **Settings** shows the new version number, but the page code itself is still the old one. Hard-refresh once with `Ctrl+F5` (`Cmd+Shift+R` on a Mac).

### Why does Settings say "This build has no update source configured"?

Automatic updates only work for official releases, because they verify the release signature with the release public key. A self-built binary has no release key; update it by replacing the binary or image yourself.

### Where is the initial password?

In the `initial-password` file in the data directory; it is never written to logs. With Docker:

```bash
docker exec <container> cat /data/initial-password
```

After signing in, change the password and add a Passkey or authenticator as soon as possible. If every sign-in method is lost, run `hostower auth reset-password` on the server.
