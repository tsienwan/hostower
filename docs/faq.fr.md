# FAQ

[简体中文](faq.md) | [繁體中文](faq.zh-TW.md) | [English](faq.en.md) | [日本語](faq.ja.md) | [한국어](faq.ko.md) | **Français** | [Deutsch](faq.de.md)

Les problèmes les plus courants et quelques conseils pour connecter des hôtes, installer Docker et installer des applications. Les noms d'hôtes et les adresses des exemples sont fictifs ; remplacez-les par les vôtres.

- [Connexion des hôtes](#connexion-des-hôtes)
- [sudo](#sudo)
- [Docker et applications conteneurisées](#docker-et-applications-conteneurisées)
- [Collecteur avancé](#collecteur-avancé)
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

### Comment sont mises à jour les images à étiquette de version fixe (comme `v3.9.0`) ?

La page Conteneurs lit la liste des étiquettes depuis le registre :

- S'il existe une version plus récente dans la même version majeure, elle affiche **Nouvelle version vX** ; la mise à jour passe à la nouvelle étiquette et recrée le conteneur avec les mêmes paramètres.
- Une version majeure plus récente est seulement signalée, jamais appliquée automatiquement : les mises à niveau majeures peuvent contenir des changements incompatibles, lisez donc d'abord les notes de version.

---

## Collecteur avancé

### Combien de temps avant que la page tienne compte d'un collecteur installé ?

Une fois la demande approuvée et exécutée avec succès, Hostower reconnecte immédiatement l'échantillonnage de l'hôte, si bien que **Collecteur avancé &lt;version&gt;** et le classement des processus apparaissent généralement en quelques secondes. Pendant l'attente, le bouton affiche **En attente d'approbation**, puis **Installation…**, puis **Changement du mode de collecte…**, pour éviter une double soumission. La désinstallation fonctionne de la même façon.

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
