// ko 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "이 호스트의 명령은 Hostower 프로세스가 직접 실행하는데, 서비스에 NoNewPrivileges(권한 상승을 막는 보안 강화)가 설정되어 sudo를 쓸 수 없어 방화벽과 Caddy를 바꿀 수 없습니다.",
  "推荐：在“总览”里把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能免密 sudo 的账号），通过它来管防火墙和 Caddy。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "권장: '개요'에서 이 머신을 SSH 방식으로 한 번 더 추가하고(주소 127.0.0.1, 암호 없이 sudo 가능한 계정) 그것으로 방화벽과 Caddy를 관리하세요. 서비스 파일의 NoNewPrivileges=true를 빼도 되지만 보안 강화가 약해집니다.",
};

export default dict;
