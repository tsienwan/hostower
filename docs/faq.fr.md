# FAQ

[简体中文](faq.md) | [繁體中文](faq.zh-TW.md) | [English](faq.en.md) | [日本語](faq.ja.md) | [한국어](faq.ko.md) | **Français** | [Deutsch](faq.de.md)

Les problèmes les plus courants et quelques conseils pour connecter des hôtes, installer Docker et installer des applications. Les noms d'hôtes et les adresses des exemples sont fictifs ; remplacez-les par les vôtres.

- [Connexion des hôtes](#connexion-des-hôtes)
- [sudo](#sudo)
- [Docker et applications conteneurisées](#docker-et-applications-conteneurisées)
- [Sites, certificats et Fail2ban](#sites-certificats-et-fail2ban)
- [Collecteur avancé](#collecteur-avancé)
- [Quotas de trafic](#quotas-de-trafic)
- [Mettre à jour Hostower](#mettre-à-jour-hostower)

---

## Connexion des hôtes

### Pourquoi un identifiant SSH ne peut-il pas utiliser root ?

Hostower détient « les clés de tous les hôtes » ; chaque hôte a donc besoin d'une barrière supplémentaire que Hostower lui-même ne peut pas ouvrir :

1. **Limiter les dégâts si le panneau est compromis** : si la base de données et la clé maîtresse fuient, un attaquant obtient les clés privées stockées par Hostower. Une clé root donne le contrôle total de l'hôte ; un compte restreint ne peut faire que ce que sudo autorise.
2. **La classification ne peut jamais être parfaite** : le moteur de risque doit analyser du shell, et il peut toujours exister un contournement auquel personne n'a pensé. Les restrictions sudo sont appliquées par le système de l'hôte lui-même — une seconde ligne de défense indépendante.
3. **Le terminal interactif n'est pas classifié commande par commande** : ouvrir un terminal nécessite une approbation L2 et la session est entièrement enregistrée, mais les commandes tapées à l'intérieur ne sont pas classifiées une à une. Connecté en root, tout est permis une fois l'approbation donnée.
4. **Bonnes pratiques d'exploitation** : beaucoup de systèmes interdisent par défaut les connexions SSH en root ; avec un compte dédié, le `auth.log` de l'hôte et le journal sudo montrent exactement ce qu'a fait Hostower.

C'est une règle stricte : le nom d'utilisateur `root` est refusé d'emblée.

### Comment créer un compte dédié pour Hostower ?

Sur l'hôte géré, en root :

```bash
useradd -m -s /bin/bash hostower
mkdir -p /home/hostower/.ssh
echo 'ssh-ed25519 AAAA… (clé publique)' >> /home/hostower/.ssh/authorized_keys
chown -R hostower:hostower /home/hostower/.ssh
chmod 700 /home/hostower/.ssh
chmod 600 /home/hostower/.ssh/authorized_keys
```

Configurez ensuite sudo (section suivante), puis ajoutez un identifiant SSH sous **Identifiants** dans Hostower : nom d'utilisateur `hostower`, et la clé privée qui correspond à la clé publique ci-dessus (tout le bloc de `-----BEGIN` à `-----END`).

### root peut déjà se connecter avec une clé. Le nouveau compte a-t-il besoin de sa propre clé ?

Oui. Les clés publiques sont propres à chaque compte : chaque compte ne fait confiance qu'aux clés de son propre `~/.ssh/authorized_keys`, et `/root/.ssh/authorized_keys` ne s'applique qu'à root.

Deux façons de faire :

- **Recommandé : générer une clé distincte pour Hostower.** Continuez à utiliser votre clé d'origine pour root ; cette clé privée reste chez vous et n'est jamais confiée à Hostower.

  ```bash
  ssh-keygen -t ed25519 -f hostower_key -N ""
  ```

  Mettez le contenu de `hostower_key.pub` dans le `authorized_keys` du compte hostower, et le contenu de `hostower_key` dans l'identifiant Hostower.
- **Rapide : copier les clés publiques de root.**

  ```bash
  cp /root/.ssh/authorized_keys /home/hostower/.ssh/
  ```

  La contrepartie : la même clé privée permet de se connecter à la fois en hostower et en root, si bien qu'une fuite depuis Hostower équivaut à une fuite de root. Si vous procédez ainsi, définissez `PermitRootLogin no` dans `/etc/ssh/sshd_config` et redémarrez sshd. Avant cela, assurez-vous de pouvoir vous connecter en hostower et passer root avec `sudo -i`, pour ne pas vous retrouver bloqué dehors.

### Je ne sais plus quelle clé publique correspond à ma clé privée

Retrouvez-la sur l'ordinateur qui détient la clé privée :

```bash
ssh-keygen -y -f fichier_cle_privee
```

### Erreur : `ssh: unable to authenticate, attempted methods [none publickey]`

La connexion réseau fonctionne, mais l'hôte n'accepte pas cette clé privée. Vérifiez, du plus courant au moins courant :

1. **La clé publique n'est pas dans le `authorized_keys` de ce compte.** Vérifiez avec `sudo cat /home/hostower/.ssh/authorized_keys` ; « No such file or directory » signifie qu'elle n'a jamais été ajoutée.
2. **Les permissions sont trop larges** : sshd ignore silencieusement ces clés. `.ssh` doit être en 700 et `authorized_keys` en 600, tous deux appartenant au compte.
3. **Mauvais nom d'utilisateur** : le nom d'utilisateur de l'identifiant doit être celui du compte qui détient la clé publique.
4. **La clé privée a été collée de façon incomplète** : la première ou la dernière ligne manque, ou des sauts de ligne ont été perdus.

Testez depuis votre propre ordinateur :

```bash
ssh -i fichier_cle_privee -o IdentitiesOnly=yes hostower@192.0.2.10
```

Si cela échoue aussi, lisez le journal de sshd sur l'hôte :

```bash
sudo journalctl -u ssh -u sshd -n 30 --no-pager
```

`bad ownership or modes` indique un problème de permissions ; `Invalid user`, un mauvais nom d'utilisateur. Si vous arrivez à vous connecter mais pas Hostower, collez de nouveau la clé privée dans **Identifiants**.

Une fois le problème corrigé, il n'y a rien d'autre à faire : Hostower se reconnecte automatiquement.

### Erreur : la clé de l'hôte ne correspond pas à celle de la première connexion

Hostower enregistre la clé d'un hôte lors de la première connexion et refuse de se connecter si elle change par la suite. Quelqu'un intercepte peut-être la connexion, ou l'hôte a peut-être été réinstallé. Une fois la réinstallation confirmée, effacez l'ancien enregistrement sur la machine qui exécute Hostower :

```bash
hostower hostkey forget web-1
```

---

## sudo

### Le compte doit-il avoir sudo ? Comment le configurer ?

Installer des paquets, installer Docker et modifier le pare-feu nécessitent tous root. Un compte non root utilise `sudo -n` (sans mot de passe ; il n'attend jamais de mot de passe). Deux options :

**Option 1 : sudo sans mot de passe pour tout (le plus simple)**

```bash
echo 'hostower ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/hostower
chmod 440 /etc/sudoers.d/hostower
visudo -c
```

Lancez toujours `visudo -c` et vérifiez que `parsed OK` s'affiche : un fichier sudoers défectueux peut rendre sudo complètement inutilisable.

Ainsi configuré, le compte peut faire presque tout ce que peut faire root. La protection vient de Hostower lui-même : chaque modification est classifiée, approuvée par vous et auditée. Par rapport à une connexion directe en root, deux avantages demeurent : les connexions SSH en root peuvent être désactivées, et les journaux montrent quelles actions viennent de Hostower.

**Option 2 : n'autoriser que certaines commandes (liste blanche)**

Convient lorsque Hostower ne doit faire que quelques actions, par exemple :

```
hostower ALL=(root) NOPASSWD: /usr/bin/systemctl, /usr/bin/journalctl
```

Notez que des commandes comme `env`, `tee`, `docker` et `sh` permettent d'exécuter n'importe quoi ou d'écrire n'importe quel fichier ; les mettre sur la liste équivaut donc à l'option 1. Des fonctionnalités comme **Installer Docker** ont justement besoin de ces commandes : elles ne fonctionnent donc qu'avec l'option 1.

**Vérifier que cela fonctionne :**

```bash
su - hostower -c 'sudo -n true && echo ok'
```

### Les systèmes anciens affichent `you must have a tty`

Sur des systèmes anciens comme CentOS 7, le fichier sudoers peut contenir `Defaults requiretty`. Désactivez-le pour ce compte :

```bash
echo 'Defaults:hostower !requiretty' >> /etc/sudoers.d/hostower
visudo -c
```

---

## Docker et applications conteneurisées

### L'installation d'une application échoue avec « cet hôte n'a pas Docker Compose v2 »

L'hôte n'a pas Docker, ou il a Docker sans le plugin `docker compose`. La boîte de dialogue d'installation d'application et la page Conteneurs vérifient désormais au préalable et, s'il manque quelque chose, affichent une note avec un bouton **Installer Docker**.

- Le bouton crée une demande (L1). Lors de l'approbation, vous voyez chaque commande assemblée pour cet hôte précis ; rien ne s'exécute avant votre approbation.
- Debian, Ubuntu, Raspbian, la famille CentOS (Rocky, AlmaLinux, RHEL) et Fedora installent depuis les dépôts officiels de Docker ; Alpine, Arch et openSUSE utilisent leurs propres dépôts. Aucun script n'est jamais téléchargé puis exécuté (`curl … | sh`).
- Si Docker est présent et que seul Compose manque, seul le plugin Compose est installé.
- Le compte doit être root ou disposer de sudo sans mot de passe (voir la section précédente) ; sinon, le bouton n'apparaît pas.
- Si le système n'est pas reconnu, installez Docker manuellement en suivant la documentation officielle de Docker.

### Après l'installation de Docker, la page Conteneurs affiche `permission denied … docker.sock`

Le compte de connexion n'est pas dans le groupe `docker`. Hostower ne s'ajoute pas lui-même à ce groupe : en faire partie équivaut à être root, seule une personne sur l'hôte doit donc l'accorder.

1. Sur l'hôte, en root (vous pouvez copier la commande depuis la page Conteneurs) :

   ```bash
   usermod -aG docker hostower
   ```

2. De retour sur la page Conteneurs, cliquez sur **Reconnecter**.

Un nouveau groupe ne prend effet qu'après une nouvelle connexion. **Reconnecter** ferme la connexion de Hostower à cet hôte et se reconnecte une fois, sans redémarrer Hostower. Attention : les commandes en cours d'exécution sur cet hôte à ce moment-là sont interrompues.

L'hôte `local` en mode hôte fait exception : il s'exécute sous l'identité du processus Hostower lui-même, vous devez donc redémarrer Hostower.

### Pourquoi les conteneurs portent-ils des noms comme `3x-ui-3x-ui-1` ?

C'est le nommage par défaut de Docker Compose (projet-service-index), que produisaient les anciennes versions. Désormais, une application à service unique nomme son conteneur d'après l'instance ; une application à plusieurs services utilise « instance-service ».

Pour les applications déjà installées, modifiez une fois leurs paramètres ou mettez-les à jour sous **Installées** : le conteneur est recréé avec le nouveau nom ; les répertoires de données ne sont pas touchés. Si l'hôte possède déjà un conteneur du même nom que Hostower n'a pas créé, le déploiement signale un conflit de nom ; occupez-vous d'abord de ce conteneur.

### L'application est installée : où l'ouvrir ? Quels sont le compte et le mot de passe initiaux ?

Sous **Applications → Installées**, cliquez sur l'icône de fusée (**Guide de démarrage**) de la ligne concernée : vous y trouvez l'adresse d'accès (cliquable directement), des explications sur le compte initial et les quelques étapes suivantes. Une fois la demande d'installation exécutée avec succès, la page de détail de la demande propose le même accès.

- Pour les applications dont le mot de passe initial peut être fixé à l'installation (mot de passe admin d'OpenList, clé maîtresse de LiteLLM, mots de passe des bases de données), le mot de passe est le secret d'application choisi lors de l'installation.
- Les applications où vous créez vous-même l'administrateur à la première ouverture (Uptime Kuma, Portainer, Open WebUI, New API, n8n) n'ont pas de compte initial ; ouvrez-les au plus vite après l'installation pour terminer l'inscription.
- Le nom d'utilisateur et le mot de passe initiaux de 3x-ui sont générés aléatoirement au premier démarrage et n'apparaissent qu'une seule fois, dans le journal du conteneur : dans le guide de démarrage, cliquez sur **Lire le nom d'utilisateur et le mot de passe initiaux**, et le panneau lit cette ligne au moyen d'une unique requête en lecture seule. Après une recréation du conteneur, le journal ne la contient plus ; dans ce cas, redéfinissez-les dans un terminal sur l'hôte avec la commande indiquée dans le guide.

Si l'adresse ne s'ouvre pas, vérifiez d'abord si l'adresse d'écoute est `127.0.0.1` (accessible uniquement depuis l'hôte lui-même ; configurez un proxy inverse ou remplacez-la par `0.0.0.0` dans **Modifier les paramètres**), puis vérifiez que le pare-feu de l'hôte et le groupe de sécurité du fournisseur cloud autorisent ce port.

### Que signifient « Restreint » et « Injoignable » sur la page « Environnement » ?

**Environnement**, sur la page de l'hôte, et **Applications → Environnement** envoient depuis cet hôte une série de requêtes en lecture seule et observent la réponse de chaque service :

- **API de grands modèles** (OpenAI, Anthropic, Google Gemini, xAI, DeepSeek) : la requête est envoyée sans clé. Une réponse « clé manquante » donne **Disponible** : il suffit de renseigner une clé pour l'appeler. Un refus motivé par le pays ou la région de la sortie donne **Restreint** : utilisez un hôte situé dans une autre région, ou passez par une passerelle telle que LiteLLM ou New API.
- **Sites web et streaming** (Google, YouTube, Netflix, Disney+, TikTok, Facebook, X, la version web de ChatGPT, etc.) : **Disponible** si le site s'ouvre normalement ; **Restreint** s'il refuse la requête ou renvoie vers une page « service non disponible dans votre région ». Pour YouTube et Netflix, la région attribuée est également affichée ; Netflix est évalué avec un titre qui n'est pas une production originale, et « Productions originales uniquement » signifie que le catalogue de cette région n'est pas ouvert à cette adresse.
- **Injoignable** : aucune connexion établie en 8 secondes ; le réseau la bloque peut-être, ou l'hôte n'a pas `curl`.
- **Incertain** : la connexion a abouti, mais la réponse ne correspond à aucun des cas ci-dessus ; impossible de conclure.

Seule la manière dont chaque service répond est examinée ici ; aucune connexion à un compte n'est effectuée. Les versions web et les connexions par abonnement de chaque fournisseur ont leurs propres règles de région et de contrôle des risques : ni une API disponible ni un site qui s'ouvre ne garantissent qu'un compte fonctionnera. La même page liste aussi les outils IA en ligne de commande installés sur l'hôte (Claude Code, Codex CLI, Gemini CLI, etc.) et les environnements d'exécution ; les versions sont lues dans les répertoires d'installation, ces programmes ne sont jamais exécutés.

### Que sont le type et le score de risque dans « IP de sortie » ? Sont-ils fiables ?

Les sites web voient l'adresse de sortie de l'hôte. Son type et ses éventuels antécédents déterminent si vous serez restreint et si vous devrez souvent passer une vérification humaine :

- **Type** : centre de données, connexion résidentielle, ligne d'entreprise, réseau mobile, etc. Un VPS a en général une adresse de centre de données ; les services de streaming n'y donnent souvent accès qu'aux productions originales, et les connexions par abonnement aux services d'IA déclenchent plus facilement le contrôle des risques.
- **Marquage proxy** : indique si la base d'informations l'a signalée comme sortie de proxy ou de VPN.
- **Score de risque** : de 0 à 100 ; plus il est élevé, plus cette adresse ou sa plage réseau compte de signalements d'abus. En dessous de 33, il est considéré comme faible ; au-dessus de 66, comme élevé.
- S'y ajoutent l'opérateur (ASN), la localisation, la présence d'une sortie IPv6 et le passage éventuel par Cloudflare WARP.

Ces informations sont obtenues par l'hôte lui-même, qui interroge une base publique d'informations IP (proxycheck.io) sur sa propre adresse de sortie ; elles ne sont données qu'à titre indicatif : les conclusions peuvent varier d'une base à l'autre, et le contrôle des risques des sites ne repose pas uniquement sur elles. Si rien n'est trouvé, « Introuvable » s'affiche, sans incidence sur les autres vérifications.

### Comment mesurer le nombre de tokens consommés par chaque outil IA ?

Installez LiteLLM ou New API (dans **Outils IA** du catalogue d'applications), faites pointer l'adresse d'API de Claude Code, Codex CLI et des autres outils vers cette passerelle, puis attribuez une clé distincte à chaque outil ou à chaque personne. Le nombre d'appels, la consommation de tokens et le coût par clé et par modèle se consultent dans l'interface d'administration de la passerelle elle-même. Hostower ne relaie ni n'enregistre les requêtes aux modèles.

### Comment sont mises à jour les images à étiquette de version fixe (comme `v3.9.0`) ?

La page Conteneurs lit la liste des étiquettes depuis le registre :

- S'il existe une version plus récente dans la même version majeure, elle affiche **Nouvelle version vX** ; la mise à jour passe à la nouvelle étiquette et recrée le conteneur avec les mêmes paramètres.
- Une version majeure plus récente est seulement signalée, jamais appliquée automatiquement : les mises à niveau majeures peuvent contenir des changements incompatibles, lisez donc d'abord les notes de version.

---

## Sites, certificats et Fail2ban

### Caddy ou Nginx ?

Sur la page « Réseau » de l'hôte, les deux s'installent en un clic (depuis les dépôts du système) et permettent les trois types de site : proxy inverse, répertoire statique et redirection.

- **Caddy** obtient et renouvelle automatiquement les certificats HTTPS : c'est le moins de travail. Choisissez-le sauf raison contraire.
- **Nginx** convient si vous connaissez Nginx ou si l'hôte a déjà une configuration Nginx. Les certificats peuvent venir de Let's Encrypt automatiquement (Hostower installe certbot au passage, et certbot renouvelle selon son propre calendrier), ou de certificats déjà présents sur l'hôte.

Les deux utilisent les ports 80 et 443 : Hostower n'en installe qu'un par hôte. Il ne gère que les fichiers qu'il écrit (`/etc/caddy/sites.d/`, `/etc/nginx/conf.d/hostower-*.conf`) ; la configuration existante de l'hôte reste intacte.

### L'obtention d'un certificat Let's Encrypt a échoué

En cas d'échec, le site est servi en HTTP en attendant, sans toucher aux autres sites. Causes fréquentes :

- Le domaine ne pointe pas encore vers cet hôte (ou le changement DNS n'est pas encore propagé).
- Le port 80 n'est pas joignable de l'extérieur : le pare-feu de l'hôte et le groupe de sécurité du fournisseur cloud doivent tous deux autoriser 80 et 443.
- Un autre programme utilise déjà le port 80.
- Trop de demandes pour le même domaine en peu de temps ont atteint les limites de Let's Encrypt (au plus 5 certificats par semaine pour un même ensemble de domaines).

Une fois le problème réglé, retirez le site puis ajoutez-le de nouveau.

### Le port 443 est déjà utilisé par 3x-ui (Xray)

Deux programmes ne peuvent pas écouter sur le même port. Déplacez l'entrée Xray sur un autre port, ou donnez au site une adresse avec port (par ex. `app.example.com:8443`) ; dans ce cas Nginx ne peut utiliser qu'un certificat existant, tandis que Caddy peut toujours en obtenir un automatiquement.

### Fail2ban peut-il me bannir, moi ou Hostower ?

La liste blanche contient toujours les adresses de bouclage et l'adresse depuis laquelle Hostower se connecte ; Hostower se connecte par clé et ne laisse donc aucun échec. Si votre IP publique change, ajoutez à la liste blanche les réseaux que vous utilisez habituellement.

Si vous êtes banni malgré tout : cliquez sur débannir à côté de l'IP dans la page « Réseau », ou connectez-vous depuis une autre adresse ou la console du fournisseur cloud et exécutez :

```bash
sudo fail2ban-client set sshd unbanip 203.0.113.7
```

Fail2ban modifie les règles du pare-feu : l'installer, changer ses réglages et débannir sont des opérations à haut risque (L2), approuvables uniquement sur le web avec une nouvelle vérification d'identité.

---

## Collecteur avancé

### Combien de temps avant que la page tienne compte d'un collecteur installé ?

Une fois la demande approuvée et exécutée avec succès, Hostower reconnecte immédiatement l'échantillonnage de l'hôte, si bien que **Collecteur avancé &lt;version&gt;** et le classement des processus apparaissent généralement en quelques secondes. Pendant l'attente, le bouton affiche **En attente d'approbation**, puis **Installation…**, puis **Changement du mode de collecte…**, pour éviter une double soumission. La désinstallation fonctionne de la même façon.

---

## Quotas de trafic

### Le trafic mensuel du panneau ne correspond pas à celui de l'hébergeur

Les chiffres de la carte « Trafic » de la page d'hôte et du « Classement du trafic » de la vue d'ensemble viennent des compteurs cumulés de chaque interface, comptés ainsi :

- **Seules les interfaces de sortie comptent.** Les tunnels (WireGuard, OpenVPN, Tailscale), les ponts (`br0`, `vmbr0`), les sous-interfaces VLAN (`eth0.100`) et les membres d'agrégat portent le même trafic et ne sont pas comptés deux fois ; les interfaces virtuelles comme `lo`, `docker0` et `veth…` sont exclues. Pour facturer sur une seule interface, choisissez-la dans « Interface comptée » sous « Définir un quota ».
- **Les jours suivent le fuseau horaire de la machine qui exécute le panneau.** Si l'hébergeur facture dans un autre fuseau, le premier et le dernier jour de la période peuvent différer de quelques heures de trafic.
- **Le trafic pendant une déconnexion ou un redémarrage du panneau est rattrapé** ; lors d'un redémarrage de l'hôte, les quelques secondes entre le dernier relevé et le redémarrage sont perdues.
- **Le trafic consommé avant le raccordement au panneau ne peut pas être mesuré.** Saisissez dans « Définir un quota » la consommation de la période affichée par l'hébergeur et la période s'y aligne. Le calibrage ne vaut que pour la période en cours et expire avec elle ; changer le mode de facturation, l'interface ou le jour de remise à zéro l'efface aussi.

La facture de l'hébergeur fait foi.

### Les débits réseau sont plus bas après la mise à jour

Les débits réseau suivent désormais la même règle que le trafic mensuel et ne comptent que les interfaces de sortie. Auparavant, le trafic des tunnels et des ponts s'ajoutait à celui de l'interface physique : le même trafic était compté deux fois et les chiffres étaient trop élevés. L'historique enregistré avant la mise à jour garde l'ancienne règle.

### Je n'ai pas reçu d'alerte de trafic

Quand un seuil d'alerte (80 %, 95 % et 100 % par défaut) est atteint, ou que le quota sera dépassé au rythme actuel, une notification part vers les canaux abonnés à « Seuil d'alerte de trafic atteint » et « Dépassement du quota de trafic prévu » ; chaque seuil n'est envoyé qu'une fois par période. Les canaux existants sont cochés automatiquement lors de la mise à jour ; si vous les avez décochés, recochez-les dans « Paramètres → Notifications et bot ». La prévision attend au moins une journée complète de données dans la période.

---

## Mettre à jour Hostower

### Les nouvelles fonctionnalités n'apparaissent pas après une mise à jour

Hostower est une application monopage. Après le redémarrage du serveur sur une nouvelle version, les pages ouvertes se reconnectent automatiquement et **Paramètres** affiche le nouveau numéro de version, mais le code de la page lui-même est toujours l'ancien. Forcez une fois l'actualisation avec `Ctrl+F5` (`Cmd+Shift+R` sur Mac).

### Pourquoi Paramètres indique-t-il « Aucune source de mise à jour n'est configurée pour cette version » ?

Les mises à jour automatiques ne fonctionnent que pour les versions officielles, car elles vérifient la signature de publication avec la clé publique de publication. Un binaire compilé soi-même n'a pas de clé de publication ; mettez-le à jour en remplaçant vous-même le binaire ou l'image.

### Où se trouve le mot de passe initial ?

Dans le fichier `initial-password` du répertoire de données ; il n'est jamais écrit dans les journaux. Avec Docker :

```bash
sudo cat data/initial-password   # à lancer dans le répertoire qui contient compose.yaml
```

Après vous être connecté, changez le mot de passe et ajoutez une Passkey ou une application d'authentification dès que possible. Si toutes les méthodes de connexion sont perdues, lancez `hostower auth reset-password` sur le serveur.
