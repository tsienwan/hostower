# FAQ

[简体中文](faq.md) | [繁體中文](faq.zh-TW.md) | [English](faq.en.md) | [日本語](faq.ja.md) | [한국어](faq.ko.md) | [Français](faq.fr.md) | **Deutsch**

Die häufigsten Probleme und Tipps beim Anbinden von Hosts, beim Installieren von Docker und beim Installieren von Apps. Hostnamen und Adressen in den Beispielen sind Platzhalter; ersetzen Sie sie durch Ihre eigenen.

- [Hosts anbinden](#hosts-anbinden)
- [sudo](#sudo)
- [Docker und Container-Apps](#docker-und-container-apps)
- [Sites, Zertifikate und Fail2ban](#sites-zertifikate-und-fail2ban)
- [Erweiterter Collector](#erweiterter-collector)
- [Traffic-Kontingente](#traffic-kontingente)
- [Ressourcenalarme](#ressourcenalarme)
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

### Die App ist installiert – wo öffne ich sie? Wie lauten anfängliches Konto und Passwort?

Klicken Sie unter **Apps → Installiert** in der betreffenden Zeile auf das Raketensymbol (**Erste Schritte**): Dort finden Sie die Zugriffsadresse (direkt anklickbar), Hinweise zum anfänglichen Konto und die nächsten Schritte. Nachdem eine Installationsanfrage erfolgreich ausgeführt wurde, bietet die Detailseite der Anfrage denselben Einstieg.

- Bei Apps, deren anfängliches Passwort bei der Installation festgelegt werden kann (admin-Passwort von OpenList, Hauptschlüssel von LiteLLM, Datenbankpasswörter), ist das Passwort das bei der Installation gewählte App-Secret.
- Apps, bei denen Sie den Administrator beim ersten Öffnen selbst anlegen (Uptime Kuma, Portainer, Open WebUI, New API, n8n), haben kein anfängliches Konto; öffnen Sie sie nach der Installation möglichst bald und schließen Sie die Registrierung ab.
- Anfänglicher Benutzername und Passwort von 3x-ui werden beim ersten Start zufällig erzeugt und erscheinen nur ein einziges Mal im Container-Log: Klicken Sie in den ersten Schritten auf **Anfänglichen Benutzernamen und Passwort auslesen**, dann liest das Panel diese Zeile mit einer einzigen Nur-Lese-Abfrage aus. Nach einer Neuerstellung des Containers steht sie nicht mehr im Log; setzen Sie die Zugangsdaten dann mit dem dort angegebenen Befehl in einem Terminal auf dem Host neu.

Lässt sich die Adresse nicht öffnen, prüfen Sie zuerst, ob die Listen-Adresse `127.0.0.1` ist (nur vom Host selbst aus erreichbar; richten Sie einen Reverse-Proxy ein oder ändern Sie sie unter **Einstellungen bearbeiten** auf `0.0.0.0`), und danach, ob Host-Firewall und Sicherheitsgruppe des Cloud-Anbieters diesen Port erlauben.

### **Panel öffnen** öffnet die falsche Adresse, oder die App lauscht nur auf `127.0.0.1`

Apps mit Web-Panel (OpenList, 3x-ui, Uptime Kuma, Portainer, LiteLLM, Open WebUI, New API, n8n) haben in ihrer Zeile unter **Installiert** eine Schaltfläche **Panel öffnen**. Standardmäßig wird die Adresse aus der aktuellen Listen-Adresse und dem Port der Instanz gebildet; ändern Sie diese unter **Einstellungen bearbeiten**, folgt die Schaltfläche automatisch. Leitet eine von Hostower auf diesem Host angelegte Site als Reverse-Proxy auf den Port der App weiter und ist keine Panel-Adresse festgelegt, öffnet die Schaltfläche die Adresse der Site. Passt das nicht, klicken Sie auf das Link-Symbol daneben und legen eine **Panel-Adresse** fest (nur der Eigentümer kann sie ändern; sie wird nur in Hostower gespeichert, ändert nichts auf dem Host, und jede Änderung wird protokolliert):

- **Direkt auf dem Host**: Port oder Pfad wurden in der App selbst geändert (zum Beispiel Port und Panel-Pfad in den „Panel-Einstellungen“ von 3x-ui), was Hostower nicht sieht. Tragen Sie hier den neuen Port und Pfad ein.
- **Vollständige Adresse**: Die App liegt hinter einem Reverse-Proxy, wird über eine Domain erreicht oder lauscht nur auf `127.0.0.1`. Tragen Sie die Adresse ein, die der Browser öffnen soll, etwa `https://panel.example.com/xui/`. Sites auf der Netzwerkseite, die bereits auf diesen Port weiterleiten, werden angeboten und lassen sich mit einem Klick übernehmen.
- **Über einen SSH-Tunnel**: Wenn Sie den Port nicht nach außen öffnen möchten, zeigt die Seite einen Befehl `ssh -N -L lokaler-Port:127.0.0.1:App-Port benutzer@host`. Führen Sie ihn auf Ihrem eigenen Rechner aus; die Schaltfläche öffnet dann `http://localhost:lokaler-Port/pfad`.

Die Panel-Adresse darf keinen Benutzernamen, kein Passwort und nichts nach `?` oder `#` enthalten – im Link der Schaltfläche landen also nie Zugangsdaten.

### Was bedeuten „Eingeschränkt“ und „Nicht erreichbar“ auf der Seite „Umgebung“?

**Umgebung** auf der Host-Seite und **Apps → Umgebung** senden von diesem Host aus eine Reihe rein lesender Anfragen und werten aus, wie die einzelnen Dienste antworten:

- **Region und Risikoprüfung der KI-Dienste**: welcher Region ChatGPT, Claude und Google (anhand der von YouTube erkannten Region) diesen Host zuordnen und ob diese Region zum üblichen Versorgungsgebiet gehört; ob die ChatGPT-Website eine Mensch-Prüfung verlangt und ob die Google-Suche die Adresse als ungewöhnlichen Traffic einstuft; ob der Standort laut IP-Datenbank mit der von den KI-Diensten erkannten Region übereinstimmt (bei einer Abweichung wird der Host oft so behandelt, als stünde er in einer nicht unterstützten Region).
- **Modell-APIs** (OpenAI, Anthropic, Google Gemini, xAI, DeepSeek): Die Anfrage wird ohne Schlüssel gesendet. Lautet die Antwort „Schlüssel fehlt“, ist die API **Verfügbar**; mit einem Schlüssel sind Aufrufe möglich. Wird die Anfrage wegen des Landes oder der Region des Ausgangs abgelehnt, ist sie **Eingeschränkt**; nehmen Sie einen Host in einer anderen Region oder leiten Sie über ein Gateway wie LiteLLM oder New API weiter.
- **Websites und Streaming** (Google, YouTube, Netflix, Disney+, TikTok, Facebook, X, die Web-Version von ChatGPT usw.): **Verfügbar**, wenn sich die Seite normal öffnen lässt; **Eingeschränkt**, wenn die Anfrage abgelehnt oder auf eine Seite „in Ihrer Region nicht verfügbar“ umgeleitet wird. Bei YouTube und Netflix wird außerdem angezeigt, in welcher Region Sie landen; Netflix wird anhand eines Titels beurteilt, der keine Eigenproduktion ist, und „Nur Originals abrufbar“ bedeutet, dass der Katalog dieser Region für diese Adresse nicht freigegeben ist.
- **Nicht erreichbar**: Innerhalb von 8 Sekunden kam keine Verbindung zustande; möglicherweise blockiert das Netzwerk sie, oder auf dem Host fehlt `curl`.
- **Unklar**: Die Verbindung kam zustande, aber die Antwort entspricht keinem der Fälle oben; eine Beurteilung ist nicht möglich.

Geprüft wird hier nur, wie die Dienste antworten; es wird bei keinem Konto angemeldet. Die Web-Versionen und Abo-Anmeldungen der Anbieter haben eigene Regeln für Regionen und Risikokontrolle; weder eine verfügbare API noch eine Website, die sich öffnen lässt, bedeutet, dass auch ein Konto funktioniert. Dieselbe Seite listet außerdem auf, welche KI-CLI-Tools auf dem Host installiert sind (Claude Code, Codex CLI, Gemini CLI usw.), sowie die Laufzeitumgebungen; die Versionen werden aus den Installationsverzeichnissen gelesen, die Programme werden nicht ausgeführt.

### Was sind Typ und Risikowert unter „Ausgangs-IP“? Wie zuverlässig sind sie?

Websites sehen die Ausgangsadresse des Hosts. Von ihrem Typ und davon, ob sie negativ aufgefallen ist, hängt ab, ob Sie eingeschränkt werden und wie oft Sie eine Mensch-Prüfung bestehen müssen:

- **Typ**: Rechenzentrum, Privatanschluss, Geschäftsanschluss, Mobilfunknetz usw. Ein VPS hat in der Regel eine Rechenzentrumsadresse; Streaming-Dienste zeigen dort oft nur Eigenproduktionen, und Abo-Anmeldungen bei KI-Diensten lösen leichter die Risikokontrolle aus.
- **Proxy-Markierung**: ob die Datenbank die Adresse als Proxy- oder VPN-Ausgang markiert hat.
- **Risikowert**: 0 bis 100; je höher, desto mehr Missbrauch ist für diese Adresse oder ihren Netzbereich verzeichnet. Unter 33 gilt als niedrig, über 66 als hoch.
- Außerdem werden der Betreiber (ASN), der Standort und die Nutzung von Cloudflare WARP angezeigt.
- IPv4 und IPv6 werden getrennt geprüft und getrennt angezeigt: Auf Hosts mit IPv6 bevorzugen viele Websites IPv6, und die beiden Ausgänge können sich in Zugehörigkeit und Reputation unterscheiden.

Diese Angaben ermittelt der Host selbst, indem er seine eigene Ausgangsadresse bei einer öffentlichen IP-Datenbank (proxycheck.io) abfragt. Sie dienen nur zur Orientierung: Verschiedene Datenbanken können zu unterschiedlichen Ergebnissen kommen, und die Risikokontrolle der Websites richtet sich nicht allein danach. Lässt sich nichts ermitteln, wird „Nicht gefunden“ angezeigt; die übrigen Prüfungen sind davon nicht betroffen.

### Was ist „Agent-Kollaboration“? Können die Assistenten meine Hosts unkontrolliert verändern?

Auf der Seite „Agent-Kollaboration“ können Sie dieselbe Aufgabe (zum Beispiel „nginx-Konfiguration und die letzten Fehlerlogs prüfen“) an Claude Code, Codex CLI oder Gemini CLI auf mehreren Hosts vergeben; der Assistent auf jedem Host arbeitet nur auf seinem eigenen Host. Danach können Sie die Ergebnisse der Hosts nebeneinander ansehen und mit „Zusammenfassen“ den Assistenten auf einem der Hosts vergleichen und zusammenfassen lassen.

- Aufgabe vergeben und Zusammenfassen sind beides Anfragen: Sie sehen zuerst den Befehl, der auf jedem Host laufen wird, und seine Risikostufe; ausgeführt wird erst nach der Genehmigung, und das Audit zeichnet wie gewohnt auf.
- Standardmäßig nur lesend: Der Assistent kann nur schauen, nichts ändern, und Aufforderungen, Befehle auszuführen, werden abgelehnt. Auch mit „Änderungen an Dateien im Arbeitsverzeichnis erlauben“ werden nur Änderungen innerhalb des Arbeitsverzeichnisses freigegeben (acceptEdits bei Claude Code, die Sandbox workspace-write bei Codex, auto_edit bei Gemini CLI).
- Die Seite bietet keinen Schalter, um Berechtigungsabfragen oder die Sandbox zu überspringen; solche Befehle, von Hand eingegeben (zum Beispiel `--dangerously-skip-permissions` oder `--yolo`), werden als Hochrisiko (L2) eingestuft.
- Der Assistent muss vorher im App-Katalog installiert und eingerichtet sein (API-Schlüssel oder Abo-Anmeldung). Hosts, auf denen er fehlt, werden angezeigt.

Die Kollaborationskarte oben auf der Seite zeigt live, was der Tower an welchen Host sendet (Farben unterscheiden Agent-Aufgaben, Dateiverteilung, App-Installation und andere Befehle), welcher Assistent gerade nachdenkt und über welche LiteLLM- bzw. New-API-Instanz die Assistenten Modelle aufrufen. Ist in den Systemeinstellungen „Bewegung reduzieren“ aktiviert, halten die Animationen der Karte an.

### Wie verbinde ich Claude Code oder Codex per MCP mit Hostower? Kann die KI ihre eigenen Anfragen genehmigen?

1. Stellen Sie unter „Einstellungen → Sicherheit → MCP-Zugang“ ein Token aus (eines pro Assistent; sein Name erscheint in Anfragen und im Audit). Das Token wird nur einmal angezeigt – speichern Sie es.
2. Hinterlegen Sie es im Assistenten. Die Einstellungsseite zeigt fertige Beispiele mit Ihrer Adresse:
   - Claude Code: `claude mcp add --transport http hostower https://hostower.example.com/mcp --header "Authorization: Bearer <Token>"`
   - Codex: in `~/.codex/config.toml` ergänzen
     ```toml
     [mcp_servers.hostower]
     url = "https://hostower.example.com/mcp"
     bearer_token_env_var = "HOSTOWER_MCP_TOKEN"
     ```

     und vor dem Start von Codex das Token in die Umgebungsvariable `HOSTOWER_MCP_TOKEN` legen.
3. Danach können Sie den Assistenten einfach bitten: „Sieh dir die Festplatte von web-1 an“ oder „Starte nginx auf web-1 neu“.

Befehle des Assistenten werden wie gewohnt eingestuft: reine Lesebefehle (L0) laufen sofort, das Ergebnis geht an den Assistenten zurück; alles andere wird zu einer Anfrage, die unter „Genehmigungen“ auf Sie wartet, während der Assistent mit `get_request` auf das Ergebnis wartet; L3 wird immer abgelehnt. Der Assistent hat keine Werkzeuge zum Genehmigen, Öffnen von Terminals, Lesen oder Schreiben von Dateien oder Ändern von Einstellungen und sieht keine Anfragen anderer. In den Anfragedetails und im Audit ist der Auslöser der Token-Name und der Zugang `mcp`.

- Der Assistent muss dort laufen, wo er die Adresse von Hostower erreicht: Ist das Panel nur lokal oder im internen Netz erreichbar, muss es der Assistent auch sein.
- Das Zertifikat der Zugriffsadresse muss für den Assistenten vertrauenswürdig sein. Ein selbstsigniertes Zertifikat, das Sie im Browser weggeklickt haben (zum Beispiel Caddys `tls internal` oder ein nur für eine IP ausgestelltes Zertifikat), lehnt der Assistent ab; das zeigt sich als Verbindungs- oder Initialisierungsfehler. Geben Sie der Zugriffsadresse eine Domain mit einem echten Zertifikat (Caddy holt es für eine Domain automatisch), oder leiten Sie den Lausch-Port von Hostower per SSH-Tunnel auf Ihren Rechner, etwa `ssh -N -L 9443:127.0.0.1:9443 <benutzer>@<server>`, und verbinden Sie den Assistenten mit `http://127.0.0.1:9443/mcp`.
- Codex liest die Token-Umgebungsvariable nur beim Start. In der Desktop-App setzen Sie `HOSTOWER_MCP_TOKEN` als Benutzer-Umgebungsvariable, beenden Codex vollständig und öffnen es neu; eine nur in einem Terminal vorübergehend gesetzte Variable sieht es nicht.
- Widerrufen Sie das Token an derselben Stelle, wenn Sie es nicht mehr brauchen; „Alle Sitzungen widerrufen“ unter „Sicherheit“ widerruft auch alle MCP-Tokens.
- Die Ausgabe von Lesebefehlen läuft über den KI-Anbieter (was wie Zugangsdaten aussieht, wird vorher maskiert). Wenn Sie das nicht möchten, stellen Sie kein Token aus. Was ein geleaktes Token kann und nicht kann, steht im [Bedrohungsmodell](threat-model.md#mcp-入口) (auf Chinesisch).

### Wie erfasse ich, wie viele Tokens die einzelnen KI-Tools verbrauchen?

Installieren Sie LiteLLM oder New API (im App-Katalog unter **KI-Tools**), lassen Sie die API-Adresse von Claude Code, Codex CLI und den anderen Tools darauf zeigen und stellen Sie für jedes Tool oder jede Person einen eigenen Schlüssel aus. Anzahl der Aufrufe, Token-Verbrauch und Kosten pro Schlüssel und pro Modell sehen Sie in der Verwaltungsoberfläche des Gateways selbst. Hostower selbst leitet Modellanfragen weder weiter noch zeichnet es sie auf.

### Wie verteile ich dieselbe Datei auf mehrere Hosts? Wie sammle ich Logs von mehreren Hosts ein?

Verwenden Sie **Dateisynchronisierung** in der linken Seitenleiste:

- **Auf Hosts verteilen**: Ziehen Sie die Dateien hinein (höchstens 20 auf einmal), geben Sie das Zielverzeichnis an und wählen Sie die Hosts aus. Aus jeder Datei wird eine Anfrage: Sie sehen zunächst, wohin auf jedem Host geschrieben wird und welche Risikostufe gilt; geschrieben wird erst nach der Genehmigung. Ein nicht vorhandenes Verzeichnis wird automatisch angelegt, eine gleichnamige Datei wird überschrieben. Für das Schreiben in Systemverzeichnisse muss das Anmeldekonto die nötigen Rechte haben; das Schreiben in sensible Pfade wie `/etc/ssh` wird als Hochrisiko eingestuft.
- **Von Hosts einsammeln**: Geben Sie das Verzeichnis und ein Dateinamensmuster an (etwa `*.log`; `*` steht für eine beliebige Zeichenfolge, `?` für ein einzelnes Zeichen), wählen Sie die Hosts und die Anzahl der Ebenen, die nach unten durchsucht werden, klicken Sie zuerst auf **Suchen**, um zu sehen, welche Dateien es gibt, und dann auf **Archiv herunterladen**. Im Archiv liegen die Dateien nach dem Schema „Hostname/Dateipfad“.

Grenzen beim Einsammeln: höchstens 200 Dateien pro Host, keine einzelne Datei größer als 100 MB und insgesamt nicht mehr als 1 GB pro Download; Dateien, die sich nicht lesen lassen (private Schlüssel, Dateien mit Zugangsdaten oder Dateien, für die dem Anmeldekonto die Rechte fehlen), und solche über den Grenzen werden im Archiv in „_跳过的文件.txt“ (übersprungene Dateien) vermerkt. Die Suche ist eine reine Leseabfrage; jede heruntergeladene Datei wird im Audit festgehalten.

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

### Die Karte „Sites“ zeigt „Caddy · Läuft nicht“

Ist er installiert, aber der Dienst läuft nicht, ist keine Site auf diesem Host erreichbar und es wird kein Zertifikat angefordert. Klicken Sie daneben auf „Caddy starten“ (für Nginx gilt dasselbe); nach der Genehmigung wird es ausgeführt. Auch beim Hinzufügen oder Entfernen einer Site wird der Dienst gestartet, falls er nicht läuft.

Schlägt der Start fehl, enthält die Ausgabe der Anfrage das aktuelle Log des Dienstes. Häufigste Ursache ist ein anderes Programm, das Port 80 oder 443 belegt, etwa ein selbst eingerichteter Caddy, Nginx oder Container auf dem Host. Beenden Sie, was den Port belegt, und starten Sie erneut.

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

## Ressourcenalarme

### Wann werden Alarme für Festplatte, Speicher und CPU gesendet?

Hostower entscheidet anhand der Messwerte, die es ohnehin erfasst; auf dem Host wird nichts zusätzlich ausgeführt:

- **Festplatte**: ein Einhängepunkt 5 Minuten lang über 90 %;
- **Speicher**: Auslastung 5 Minuten lang über 90 % (Puffer und Cache zählen nicht als belegt);
- **CPU**: Auslastung 10 Minuten lang über 90 %.

Fällt der Wert zwischendurch einmal unter 90 %, beginnt die Zeitmessung neu. Nach einem Alarm wird dasselbe nicht erneut gemeldet, bis die Auslastung wieder sinkt (Festplatte unter 85 %, Speicher und CPU unter 80 %) und 5 Minuten dort bleibt; dann folgt eine Entwarnung. Solange ein Host offline ist, wird nichts bewertet, und ein Neustart des Panels sendet noch offene Alarme nicht erneut. Die Schwellen sind fest und lassen sich nicht ändern.

Die Benachrichtigungen gehen an Kanäle, die „Festplatte fast voll“, „Hohe Speicherauslastung“ und „Hohe CPU-Auslastung“ abonniert haben. Bestehende Kanäle bekommen beim Update alle drei Häkchen; entfernen Sie sie unter „Einstellungen → Benachrichtigungen & Bot“, wenn Sie sie nicht möchten.

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
