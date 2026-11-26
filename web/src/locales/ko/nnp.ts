// ko 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "这台主机上的命令由 Hostower 进程直接执行，而服务设置了 NoNewPrivileges（禁止提权的安全加固），所以 sudo 用不了，改不了防火墙和 Caddy。": "이 호스트의 명령은 Hostower 프로세스가 직접 실행하는데, 서비스에 NoNewPrivileges(권한 상승을 막는 보안 강화)가 설정되어 sudo를 쓸 수 없어 방화벽과 Caddy를 바꿀 수 없습니다.",
  "推荐：点右上角的“编辑”，把这台主机的连接方式改成 SSH（地址 127.0.0.1，选一条能免密 sudo 的账号的凭据）。它还是这一台，不用另外添加主机。也可以去掉服务文件里的 NoNewPrivileges=true，但那会削弱加固。": "권장: 오른쪽 위의 '편집'을 눌러 이 호스트의 연결 방식을 SSH로 바꾸세요(주소 127.0.0.1, 암호 없이 sudo가 가능한 계정의 자격 증명). 호스트는 그대로 한 대이며 따로 추가할 필요가 없습니다. 서비스 파일의 NoNewPrivileges=true를 빼도 되지만 보안 강화가 약해집니다.",
};

export default dict;
