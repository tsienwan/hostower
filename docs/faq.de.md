# FAQ

[简体中文](faq.md) | [繁體中文](faq.zh-TW.md) | [English](faq.en.md) | [日本語](faq.ja.md) | [한국어](faq.ko.md) | [Français](faq.fr.md) | **Deutsch**

Die häufigsten Probleme und Tipps beim Anbinden von Hosts, beim Installieren von Docker und beim Installieren von Apps. Hostnamen und Adressen in den Beispielen sind Platzhalter; ersetzen Sie sie durch Ihre eigenen.

- [Hosts anbinden](#hosts-anbinden)
- [sudo](#sudo)
- [Docker und Container-Apps](#docker-und-container-apps)
- [Sites, Zertifikate und Fail2ban](#sites-zertifikate-und-fail2ban)
- [Erweiterter Collector](#erweiterter-collector)
- [Traffic-Kontingente](#traffic-kontingente)
- [Hostower aktualisieren](#hostower-aktualisieren)

---

## Hosts anbinden

### Warum dürfen SSH-Zugangsdaten nicht root verwenden?

Hostower hält „die Schlüssel zu allen Hosts“, deshalb braucht jeder Host ein weiteres Tor, das Hostower selbst nicht öffnen kann:

1. **Den Schaden begrenzen, falls das Panel kompromittiert wird**: Gelangen Datenbank und Hauptschlüssel nach außen, erhält ein Angreifer die privaten Schlüssel, die Hostower speichert. Ein root-Schlüssel bedeutet volle Kontrolle über den Host; ein eingeschränktes Konto kann nur tun, was sudo erlaubt.
2. **Die Einstufung kann nie perfekt sein**: Die Risiko-Engine muss Shell parsen, und es kann immer eine Umgehung geben, an die niemand gedacht hat. sudo-Einschränkungen werden vom System des Hosts selbst durchgesetzt – eine unabhängige zweite Verteidigungslinie.
3. **Das interaktive Terminal wird nicht Befehl für Befehl eingestuft**: Das Öffnen eines Terminals erfordert eine L2-Genehmigung und wird vollständig aufgezeichnet, aber die darin eingegebenen Befehle werden nicht einzeln eingestuft. Als root angemeldet ist nach der Genehmigung alles möglich.
4. **Betriebspraxis**: Viele Systeme verbieten root-Anmeldungen per SSH standardmäßig; mit einem eigenen Konto zeigen `auth.log` und das sudo-Protokoll des Hosts genau, was Hostower getan hat.

Das ist eine feste Regel: Der Benutzername `root` wird sofort abgelehnt.

### Wie lege ich ein eigenes Konto für Hostower an?

Auf dem verwalteten Host, als root:

```bash
useradd -m -s /bin/bash hostower
mkdir -p /home/hostower/.ssh
echo 'ssh-ed25519 AAAA… (public key)' >> /home/hostower/.ssh/authorized_keys
chown -R hostower:hostower /home/hostower/.ssh
chmod 700 /home/hostower/.ssh
chmod 600 /home/hostower/.ssh/authorized_keys
```

Richten Sie dann sudo ein (nächster Abschnitt) und fügen Sie in Hostower unter **Zugangsdaten** SSH-Zugangsdaten hinzu: Benutzername `hostower` und der private Schlüssel, der zum obigen öffentlichen Schlüssel passt (der gesamte Block von `-----BEGIN` bis `-----END`).

### root kann sich bereits mit einem Schlüssel anmelden. Braucht das neue Konto eine eigene Schlüsseleinrichtung?

Ja. Öffentliche Schlüssel gelten pro Konto: Jedes Konto vertraut nur den Schlüsseln in seiner eigenen `~/.ssh/authorized_keys`, und `/root/.ssh/authorized_keys` gilt nur für root.

Zwei Möglichkeiten:

- **Empfohlen: einen eigenen Schlüssel für Hostower erzeugen.** Verwenden Sie für root weiterhin Ihren ursprünglichen Schlüssel; dieser private Schlüssel bleibt bei Ihnen und wird nie an Hostower übergeben.

  ```bash
  ssh-keygen -t ed25519 -f hostower_key -N ""
  ```

  Tragen Sie den Inhalt von `hostower_key.pub` in die `authorized_keys` des Kontos hostower ein und den Inhalt von `hostower_key` in die Zugangsdaten in Hostower.
- **Schnell: die öffentlichen Schlüssel von root kopieren.**

  ```bash
  cp /root/.ssh/authorized_keys /home/hostower/.ssh/
  ```

  Der Preis: Mit demselben privaten Schlüssel kann man sich sowohl als hostower als auch als root anmelden, ein Leck bei Hostower ist also ein root-Leck. Wenn Sie so vorgehen, setzen Sie `PermitRootLogin no` in `/etc/ssh/sshd_config` und starten Sie sshd neu. Stellen Sie vorher sicher, dass Sie sich als hostower anmelden und mit `sudo -i` zu root wechseln können, damit Sie sich nicht selbst aussperren.

### Ich weiß nicht mehr, welcher öffentliche Schlüssel zu meinem privaten Schlüssel gehört

Leiten Sie ihn auf dem Computer ab, auf dem der private Schlüssel liegt:

```bash
ssh-keygen -y -f private_key_file
```

### Fehler: `ssh: unable to authenticate, attempted methods [none publickey]`

Die Netzwerkverbindung funktioniert, aber der Host akzeptiert diesen privaten Schlüssel nicht. Prüfen Sie, vom häufigsten zum seltensten Fall:

1. **Der öffentliche Schlüssel steht nicht in den `authorized_keys` dieses Kontos.** Prüfen Sie es mit `sudo cat /home/hostower/.ssh/authorized_keys`; „No such file or directory“ bedeutet, dass er nie hinzugefügt wurde.
2. **Die Berechtigungen sind zu offen**: sshd ignoriert solche Schlüssel stillschweigend. `.ssh` muss 700 sein und `authorized_keys` 600, beide im Besitz des Kontos.
3. **Falscher Benutzername**: Der Benutzername in den Zugangsdaten muss das Konto sein, in dem der öffentliche Schlüssel liegt.
4. **Der private Schlüssel wurde unvollständig eingefügt**: Die erste oder letzte Zeile fehlt, oder Zeilenumbrüche gingen verloren.

Testen Sie von Ihrem eigenen Computer aus:

```bash
ssh -i private_key_file -o IdentitiesOnly=yes hostower@192.0.2.10
```

Schlägt auch das fehl, lesen Sie das sshd-Protokoll auf dem Host:

```bash
sudo journalctl -u ssh -u sshd -n 30 --no-pager
```

`bad ownership or modes` bedeutet ein Berechtigungsproblem; `Invalid user` bedeutet einen falschen Benutzernamen. Wenn Sie sich anmelden können, Hostower aber nicht, fügen Sie den privaten Schlüssel unter **Zugangsdaten** erneut ein.

Ist das Problem behoben, ist nichts weiter zu tun: Hostower verbindet sich automatisch neu.

### Fehler: Der Host-Schlüssel stimmt nicht mit dem der ersten Verbindung überein

Hostower speichert den Schlüssel eines Hosts bei der ersten Verbindung und verweigert die Verbindung, wenn er sich später ändert. Möglicherweise fängt jemand die Verbindung ab, oder der Host wurde neu installiert. Sobald Sie eine Neuinstallation bestätigt haben, löschen Sie den alten Eintrag auf dem Rechner, auf dem Hostower läuft:

```bash
hostower hostkey forget web-1
```

---

## sudo

### Sollte das Konto sudo haben? Wie richte ich es ein?

Pakete installieren, Docker installieren und die Firewall ändern erfordern root. Ein Konto ohne root-Rechte verwendet `sudo -n` (ohne Passwort; es wartet nie auf eine Passworteingabe). Zwei Möglichkeiten:

**Möglichkeit 1: sudo ohne Passwort für alles (am einfachsten)**

```bash
echo 'hostower ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/hostower
chmod 440 /etc/sudoers.d/hostower
visudo -c
```

Führen Sie immer `visudo -c` aus und achten Sie auf `parsed OK`: Eine fehlerhafte sudoers-Datei kann sudo vollständig unbrauchbar machen.

Damit kann das Konto nahezu alles, was root kann. Der Schutz kommt von Hostower selbst: Jede Änderung wird eingestuft, von Ihnen genehmigt und auditiert. Gegenüber einer direkten Anmeldung als root bleiben zwei Vorteile: root-Anmeldungen per SSH lassen sich deaktivieren, und die Protokolle zeigen, welche Aktionen von Hostower stammen.

**Möglichkeit 2: nur bestimmte Befehle erlauben (Allowlist)**

Geeignet, wenn Hostower nur wenige Dinge tun soll, zum Beispiel:

```
hostower ALL=(root) NOPASSWD: /usr/bin/systemctl, /usr/bin/journalctl
```

Beachten Sie, dass Befehle wie `env`, `tee`, `docker` und `sh` es erlauben, beliebige Befehle auszuführen oder beliebige Dateien zu schreiben; sie auf die Liste zu setzen, entspricht also Möglichkeit 1. Funktionen wie **Docker installieren** benötigen genau diese Befehle und funktionieren daher nur mit Möglichkeit 1.

**Prüfen, ob es funktioniert:**

```bash
su - hostower -c 'sudo -n true && echo ok'
```

### Ältere Systeme melden `you must have a tty`

Auf älteren Systemen wie CentOS 7 kann sudoers `Defaults requiretty` enthalten. Schalten Sie es für dieses Konto ab:

```bash
echo 'Defaults:hostower !requiretty' >> /etc/sudoers.d/hostower
visudo -c
```

---

## Docker und Container-Apps

### Die Installation einer App schlägt mit „Dieser Host hat kein Docker Compose v2“ fehl

Der Host hat kein Docker, oder er hat Docker ohne das Plugin `docker compose`. Der Dialog zur App-Installation und die Seite **Container** prüfen das jetzt vorab und zeigen, falls etwas fehlt, einen Hinweis mit einer Schaltfläche **Docker installieren**.

- Die Schaltfläche erstellt eine Anfrage (L1). Bei der Genehmigung sehen Sie jeden Befehl, der für diesen konkreten Host zusammengestellt wurde; nichts wird ausgeführt, bevor Sie genehmigen.
- Debian, Ubuntu, Raspbian, die CentOS-Familie (Rocky, AlmaLinux, RHEL) und Fedora installieren aus den offiziellen Docker-Paketquellen; Alpine, Arch und openSUSE verwenden ihre eigenen Paketquellen. Es wird nie ein Skript heruntergeladen und ausgeführt (`curl … | sh`).
- Ist Docker vorhanden und fehlt nur Compose, wird nur das Compose-Plugin installiert.
- Das Konto muss root sein oder sudo ohne Passwort haben (siehe vorheriger Abschnitt); andernfalls gibt es keine Schaltfläche.
- Wird das System nicht erkannt, installieren Sie Docker manuell nach der offiziellen Docker-Dokumentation.

### Nach der Docker-Installation meldet die Seite „Container“ `permission denied … docker.sock`

Das Anmeldekonto ist nicht in der Gruppe `docker`. Hostower fügt sich nicht selbst dieser Gruppe hinzu: Die Mitgliedschaft entspricht root-Rechten, daher sollte sie nur ein Mensch auf dem Host vergeben.

1. Auf dem Host, als root (Sie können den Befehl von der Seite **Container** kopieren):

   ```bash
   usermod -aG docker hostower
   ```

2. Klicken Sie zurück auf der Seite **Container** auf **Neu verbinden**.

Eine neue Gruppe wird erst nach einer erneuten Anmeldung wirksam. **Neu verbinden** schließt die Verbindung von Hostower zu diesem Host und meldet sich erneut an, ohne Hostower neu zu starten. Hinweis: Befehle, die in diesem Moment auf diesem Host laufen, werden unterbrochen.

Der `local`-Host im Host-Modus ist eine Ausnahme: Er läuft unter der Identität des Hostower-Prozesses selbst, daher müssen Sie Hostower neu starten.

### Warum heißen Container zum Beispiel `3x-ui-3x-ui-1`?

Das ist die Standardbenennung von Docker Compose (Projekt-Dienst-Index), die ältere Versionen erzeugt haben. Jetzt benennt eine App mit nur einem Dienst ihren Container nach der Instanz; eine App mit mehreren Diensten verwendet „Instanz-Dienst“.

Bei bereits installierten Apps bearbeiten Sie einmal deren Einstellungen oder aktualisieren sie unter **Installiert**, dann wird der Container mit dem neuen Namen neu erstellt; Datenverzeichnisse bleiben unberührt. Hat der Host bereits einen gleichnamigen Container, den nicht Hostower erstellt hat, meldet die Bereitstellung einen Namenskonflikt; kümmern Sie sich zuerst um diesen Container.

### Wie werden Images mit festen Versions-Tags (etwa `v3.9.0`) aktualisiert?

Die Seite **Container** liest die Tag-Liste aus der Registry:

- Gibt es innerhalb derselben Hauptversion eine neuere Version, wird **Neue Version vX** angezeigt; beim Aktualisieren wird auf den neuen Tag gewechselt und der Container mit denselben Einstellungen neu erstellt.
- Eine neuere Hauptversion wird nur erwähnt, nie automatisch übernommen: Upgrades auf eine neue Hauptversion können inkompatible Änderungen enthalten, lesen Sie daher zuerst die Versionshinweise.

---

## Sites, Zertifikate und Fail2ban

### Caddy oder Nginx?

Auf der Seite „Netzwerk“ des Hosts lassen sich beide mit einem Klick installieren (aus den Systempaketquellen), und beide bieten die drei Site-Typen Reverse-Proxy, statisches Verzeichnis und Weiterleitung.

- **Caddy** holt und erneuert HTTPS-Zertifikate automatisch und macht am wenigsten Arbeit. Nehmen Sie es, wenn nichts dagegen spricht.
- **Nginx** passt, wenn Sie Nginx kennen oder auf dem Host schon Nginx-Konfiguration liegt. Zertifikate kommen automatisch von Let's Encrypt (Hostower installiert certbot mit, certbot erneuert nach eigenem Zeitplan) oder aus vorhandenen Zertifikaten auf dem Host.

Beide brauchen die Ports 80 und 443, daher installiert Hostower pro Host nur einen. Es verwaltet nur die Dateien, die es selbst schreibt (`/etc/caddy/sites.d/`, `/etc/nginx/conf.d/hostower-*.conf`); vorhandene Konfiguration auf dem Host bleibt unberührt.

### Das Let's-Encrypt-Zertifikat ließ sich nicht beziehen

Schlägt es fehl, wird die Site vorerst per HTTP ausgeliefert, andere Sites bleiben unberührt. Häufige Ursachen:

- Die Domain zeigt noch nicht auf diesen Host (oder die DNS-Änderung ist noch nicht verteilt).
- Port 80 ist von außen nicht erreichbar: Host-Firewall und Sicherheitsgruppe des Cloud-Anbieters müssen beide 80 und 443 erlauben.
- Ein anderes Programm belegt Port 80.
- Zu viele Anfragen für dieselbe Domain in kurzer Zeit haben die Limits von Let's Encrypt erreicht (höchstens 5 Zertifikate pro Woche für dieselben Domains).

Danach die Site entfernen und neu anlegen.

### Port 443 wird schon von 3x-ui (Xray) belegt

Zwei Programme können nicht auf demselben Port lauschen. Verlegen Sie den Xray-Inbound auf einen anderen Port, oder geben Sie der Site eine Adresse mit Port (z. B. `app.example.com:8443`); dann kann Nginx nur ein vorhandenes Zertifikat nutzen, Caddy holt weiterhin automatisch eines.

### Kann Fail2ban mich oder Hostower aussperren?

Auf der Positivliste stehen immer die Loopback-Adressen und die Adresse, von der Hostower verbindet; Hostower meldet sich per Schlüssel an und hinterlässt keine Fehlversuche. Wenn sich Ihre eigene öffentliche IP ändert, tragen Sie die Netze ein, aus denen Sie üblicherweise kommen.

Wurden Sie trotzdem gesperrt: auf der Seite „Netzwerk“ neben der IP auf Entsperren klicken, oder sich von einer anderen Adresse bzw. über die Konsole des Cloud-Anbieters anmelden und ausführen:

```bash
sudo fail2ban-client set sshd unbanip 203.0.113.7
```

Fail2ban ändert Firewall-Regeln, daher sind Installation, Änderungen an den Einstellungen und Entsperren Hochrisiko-Vorgänge (L2): Sie lassen sich nur im Web und mit erneuter Identitätsprüfung genehmigen.

---

## Erweiterter Collector

### Wie lange dauert es, bis die Seite einen installierten Collector anzeigt?

Sobald die Anfrage genehmigt und erfolgreich ausgeführt wurde, verbindet Hostower die Messwerterfassung des Hosts sofort neu, sodass **Erweiterter Collector &lt;Version&gt;** und die Prozessrangliste meist innerhalb von Sekunden erscheinen. Während des Wartens zeigt die Schaltfläche **Wartet auf Genehmigung**, dann **Wird installiert…** und dann **Erfassungsmodus wird umgestellt…**, sodass sie nicht doppelt abgeschickt werden kann. Die Deinstallation funktioniert genauso.

---

## Traffic-Kontingente

### Der Monats-Traffic im Panel weicht vom Anbieter-Panel ab

Die Zahlen auf der Karte „Traffic“ der Host-Seite und in der „Traffic-Rangliste“ der Übersicht stammen aus den kumulierten Zählern der Schnittstellen und werden so gezählt:

- **Nur Uplink-Schnittstellen zählen.** Tunnel wie WireGuard, OpenVPN und Tailscale, Bridges wie `br0` und `vmbr0`, VLAN-Unterschnittstellen wie `eth0.100` und Bond-Mitglieder tragen denselben Traffic und werden nicht doppelt gezählt; virtuelle Schnittstellen wie `lo`, `docker0` und `veth…` sind ausgenommen. Soll nur eine Schnittstelle abgerechnet werden, wählen Sie sie unter „Kontingent festlegen“ bei „Gezählte Schnittstelle“.
- **Tage richten sich nach der Zeitzone des Rechners, auf dem das Panel läuft.** Rechnet der Anbieter in einer anderen Zeitzone ab, können erster und letzter Tag eines Zeitraums um einige Stunden Traffic abweichen.
- **Traffic während einer Verbindungsunterbrechung oder eines Neustarts des Panels wird nachgeholt**; startet der Host neu, fehlen die wenigen Sekunden zwischen der letzten Messung und dem Neustart.
- **Traffic vor der Anbindung an das Panel lässt sich nicht messen.** Tragen Sie unter „Kontingent festlegen“ den vom Anbieter angezeigten Verbrauch dieses Zeitraums ein, dann wird der Zeitraum daran ausgerichtet. Die Angleichung gilt nur für den aktuellen Zeitraum und läuft mit ihm ab; eine Änderung von Abrechnungsart, Schnittstelle oder Rücksetztag löscht sie ebenfalls.

Maßgeblich ist die Rechnung des Anbieters.

### Nach dem Update sind die Netzwerkraten niedriger

Netzwerkraten folgen jetzt derselben Regel wie der Monats-Traffic und zählen nur Uplink-Schnittstellen. Vorher kam der Traffic von Tunneln und Bridges zu dem der physischen Schnittstelle hinzu, derselbe Traffic wurde also doppelt gezählt und die Werte waren zu hoch. Vor dem Update gespeicherte Verläufe nutzen weiterhin die alte Regel.

### Ich habe keine Traffic-Warnung erhalten

Wird eine Warnschwelle (standardmäßig 80 %, 95 % und 100 %) erreicht oder wird das Kontingent beim aktuellen Tempo voraussichtlich überschritten, geht eine Benachrichtigung an die Kanäle, die „Traffic-Warnschwelle erreicht“ und „Traffic-Kontingent wird voraussichtlich überschritten“ abonniert haben; jede Stufe wird pro Zeitraum nur einmal gesendet. Bestehende Kanäle bekommen beim Update beide Häkchen; falls Sie sie entfernt haben, setzen Sie sie unter „Einstellungen → Benachrichtigungen & Bot“ erneut. Die Prognose braucht mindestens einen vollen Tag Daten im Zeitraum.

---

## Hostower aktualisieren

### Neue Funktionen erscheinen nach einem Update nicht

Hostower ist eine Single-Page-App. Nachdem der Server mit einer neuen Version neu gestartet ist, verbinden sich geöffnete Seiten automatisch neu und **Einstellungen** zeigt die neue Versionsnummer, aber der Code der Seite selbst ist noch der alte. Laden Sie die Seite einmal mit `Ctrl+F5` (`Cmd+Shift+R` auf einem Mac) vollständig neu.

### Warum zeigt „Einstellungen“ „Für diesen Build ist keine Update-Quelle konfiguriert“?

Automatische Updates funktionieren nur für offizielle Releases, weil sie die Release-Signatur mit dem öffentlichen Release-Schlüssel prüfen. Ein selbst gebautes Binärprogramm hat keinen Release-Schlüssel; aktualisieren Sie es, indem Sie das Binärprogramm oder Image selbst ersetzen.

### Wo ist das Anfangspasswort?

In der Datei `initial-password` im Datenverzeichnis; es wird nie in Protokolle geschrieben. Mit Docker:

```bash
sudo cat data/initial-password   # im Verzeichnis mit der compose.yaml ausführen
```

Ändern Sie nach der Anmeldung so bald wie möglich das Passwort und fügen Sie einen Passkey oder einen Authenticator hinzu. Wenn alle Anmeldemethoden verloren sind, führen Sie auf dem Server `hostower auth reset-password` aus.
