// ko 词典：本机主机禁止提权（NoNewPrivileges）时的说明。
const dict: Record<string, string> = {
  "执行命令的进程被禁止提权（服务设置了 NoNewPrivileges），sudo 用不了，{a}。可以把这台机器以 SSH 方式再添加一次（地址 127.0.0.1，用一个能 sudo 的账号），通过它来管": "명령을 실행하는 프로세스가 권한 상승을 금지당했습니다(서비스에 NoNewPrivileges). sudo를 쓸 수 없어 {a}. 이 머신을 SSH 방식으로 다시 추가하고(주소 127.0.0.1, sudo 가능한 계정) 그것으로 관리하세요",
  "登录账号不是 root，也没有 sudo：{a}": "로그인 계정이 root가 아니고 sudo도 없습니다: {a}",
  "改不了防火墙": "방화벽을 바꿀 수 없습니다",
  "改不了 Caddy 的配置": "Caddy 설정을 바꿀 수 없습니다",
  "装不了 Caddy": "Caddy를 설치할 수 없습니다",
};

export default dict;
