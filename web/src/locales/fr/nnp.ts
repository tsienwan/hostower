// fr 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "Sur cet hôte, les commandes sont exécutées directement par le processus Hostower, et le service définit NoNewPrivileges (durcissement interdisant l’élévation de privilèges) : sudo ne peut pas fonctionner, pare-feu et Caddy non modifiables.",
  "推荐：点右上角的“编辑”，把这台主机的连接方式改成 SSH（地址 127.0.0.1，选一条能免密 sudo 的账号的凭据）。它还是这一台，不用另外添加主机。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "Recommandé : cliquez sur Modifier en haut à droite et passez la connexion de cet hôte en SSH (adresse 127.0.0.1, identifiant d’un compte avec sudo sans mot de passe). Il reste le même hôte : inutile d’en ajouter un autre. Vous pouvez aussi retirer NoNewPrivileges=true du fichier de service, mais cela affaiblit le durcissement.",
};

export default dict;
