// fr 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "Sur cet hôte, les commandes sont exécutées directement par le processus Hostower, et le service définit NoNewPrivileges (durcissement interdisant l’élévation de privilèges) : sudo ne peut pas fonctionner, pare-feu et Caddy non modifiables.",
  "推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "Recommandé : dans Vue d’ensemble, ajoutez à nouveau cette machine via SSH (adresse 127.0.0.1, compte avec sudo sans mot de passe) et gérez par lui pare-feu et Caddy. Vous pouvez aussi retirer NoNewPrivileges=true du fichier de service, mais cela affaiblit le durcissement.",
};

export default dict;
