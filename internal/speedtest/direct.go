package speedtest

import (
	"errors"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// “这台设备 ↔ 任意一台主机”的直连测速。
//
// 浏览器不能从面板的 HTTPS 页面去连别的主机，所以在选中的主机上临时开一个只活一两分钟的测速服务：
// 浏览器另开一页直接连它（HTTP，路径里带一次性的口令），量延迟、下载、上传，把数字交给这个服务，
// 服务把它们和它看到的来源地址打印出来后退出；随后同一条命令从主机追踪到那个地址。
// 整条命令照常走内核：判级、批准、执行、审计。结果从命令的输出里解析，和别的测速一样。

// MethodDirect 是“这台设备 ↔ 主机”的直连测速。
const MethodDirect = "direct"

// DirectPort 是临时测速服务默认用的端口；主机的防火墙或云厂商的安全组要放行它。
const DirectPort = 28080

const (
	directMark = "echo 'speedtest direct port="
	// DirectWait 是服务等浏览器连上来的秒数，DirectRun 是连上之后最多再跑的秒数；都写死在脚本里。
	DirectWait = 120
	DirectRun  = 75
)

var directTokenRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ValidDirectPort 报告端口能不能用来开临时测速服务（不占系统端口）。
func ValidDirectPort(p int) bool { return p >= 1024 && p <= 65535 }

func validClientSize(mb int) bool {
	for _, s := range ClientSizes {
		if s == mb {
			return true
		}
	}
	return false
}

// directScript 是在主机上跑的临时测速服务（python3，只用标准库）。里面不能有单引号和反引号：它整个放在命令的单引号里。
// 参数：端口、口令、数据量上限（MB）。它只认路径以口令开头的请求，只认第一个带对口令的来源地址，
// 下载不超过上限、上传不超过上限的四分之一，没人来就在 DirectWait 秒后退出，有人来也最多再跑 DirectRun 秒。
const directScript = `import sys,os,json,time,threading,socket,socketserver,re,math
from http.server import BaseHTTPRequestHandler
PORT=int(sys.argv[1]);TOK=sys.argv[2];MB=int(sys.argv[3]);CAP=MB*1000000
PAGE=rb"""<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><meta name=referrer content=no-referrer><title>Hostower</title>
<style>body{font:16px system-ui,sans-serif;margin:24px auto;padding:0 20px;max-width:420px;color:#222;background:#fafafa}@media(prefers-color-scheme:dark){body{color:#eee;background:#111}}h1{font-size:18px}.r{display:flex;justify-content:space-between;padding:12px 0;border-bottom:1px solid #8884}b{font-variant-numeric:tabular-nums}p{color:#888;font-size:14px}</style>
<h1 id=t></h1><div class=r><span id=lp></span><b id=p>-</b></div><div class=r><span id=ld></span><b id=d>-</b></div><div class=r><span id=lu></span><b id=u>-</b></div><p id=s></p>
<script>(async()=>{
const $=i=>document.getElementById(i);let L={};
try{L=JSON.parse(decodeURIComponent(location.hash.slice(1)))}catch(e){}
const T=(k,d)=>typeof L[k]=="string"&&L[k]?L[k].slice(0,120):d;
$("t").textContent=T("title","Speed test");$("lp").textContent=T("ping","Latency");$("ld").textContent=T("down","Download");$("lu").textContent=T("up","Upload");$("s").textContent=T("run","Testing...");
const MB=__MB__,SEC=10,now=()=>performance.now();
const show=(i,n,ms)=>{$(i).textContent=(ms>0?n*8/ms/1000:0).toFixed(1)+" Mbps"};
const pings=[];let lost=0;
for(let i=0;i<22;i++){const a=now();try{const r=await fetch("ping?"+i,{cache:"no-store"});if(!r.ok)throw 0;if(i>1)pings.push(now()-a)}catch(e){if(i>1)lost++}}
const s=[...pings].sort((a,b)=>a-b);$("p").textContent=s.length?s[s.length>>1].toFixed(1)+" ms":"-";
async function down(){const C=4,ac=new AbortController(),a=now();let n=0;
const tm=setTimeout(()=>ac.abort(),SEC*1000),tk=setInterval(()=>show("d",n,now()-a),300);
await Promise.all([...Array(C)].map(async(_,i)=>{try{const r=await fetch("down?mb="+Math.ceil(MB/C)+"&i="+i,{cache:"no-store",signal:ac.signal});if(!r.ok)return;const rd=r.body.getReader();for(;;){const x=await rd.read();if(x.done)break;n+=x.value.length}}catch(e){}}));
clearTimeout(tm);clearInterval(tk);const ms=now()-a;show("d",n,ms);return{bytes:n,ms:ms,conns:C}}
async function up(){const C=2,a=now(),cap=MB*250000;let n=0,sent=0,size=262144;
const buf=new Uint8Array(4194304);for(let o=0;o<buf.length;o+=65536)crypto.getRandomValues(buf.subarray(o,o+65536));
const tk=setInterval(()=>show("u",n,now()-a),300);
await Promise.all([...Array(C)].map(async()=>{while(now()-a<SEC*1000&&sent<cap){const k=Math.min(size,cap-sent);sent+=k;const b=now();try{const r=await fetch("up",{method:"POST",body:buf.subarray(0,k)});if(!r.ok)break;n+=k}catch(e){break}if(now()-b<300&&size<4194304)size*=2}}));
clearInterval(tk);const ms=now()-a;show("u",n,ms);return{bytes:n,ms:ms,conns:C}}
const dn=await down(),u=await up();
let proto="",conn="";try{proto=performance.getEntriesByType("navigation")[0].nextHopProtocol||""}catch(e){}
try{conn=(navigator.connection&&navigator.connection.type)||""}catch(e){}
let ok=false;try{ok=(await fetch("done",{method:"POST",body:JSON.stringify({pings:pings,lost:lost,proto:proto,conn:conn,down:dn,up:u})})).ok}catch(e){}
$("s").textContent=ok?T("done","Done. You can close this page."):T("fail","Could not hand the result back.");
if(ok)setTimeout(()=>window.close(),1500)})()</script>""".replace(b"__MB__",str(MB).encode())
BLOB=os.urandom(1<<20)
lock=threading.Lock()
st={"ip":None,"first":0,"down":CAP,"up":CAP//4,"done":False}
def out(s):
    sys.stdout.write("speedtest "+s+"\n");sys.stdout.flush()
def fnum(v,top):
    v=float(v)
    if not math.isfinite(v) or v<0 or v>top:raise ValueError
    return v
class H(BaseHTTPRequestHandler):
    protocol_version="HTTP/1.1"
    def log_message(self,*a):pass
    def part(self):
        ip=self.client_address[0]
        if ip.startswith("::ffff:"):ip=ip[7:]
        ip=ip.split("%")[0]
        p=self.path.split("?")[0]
        if not p.startswith("/"+TOK+"/"):return None
        with lock:
            if st["ip"] is None:
                st["ip"]=ip;st["first"]=time.time();out("client ip="+ip)
            elif st["ip"]!=ip:return None
        return p[len(TOK)+2:]
    def reply(self,code,body=b"",ct="text/plain"):
        self.send_response(code);self.send_header("Content-Type",ct);self.send_header("Content-Length",str(len(body)))
        self.send_header("Cache-Control","no-store");self.send_header("X-Content-Type-Options","nosniff");self.end_headers();self.wfile.write(body)
    def do_GET(self):
        p=self.part()
        if p is None:self.close_connection=True;return self.reply(404)
        if p=="":return self.reply(200,PAGE,"text/html; charset=utf-8")
        if p=="ping":return self.reply(200,b"ok")
        if p=="down":
            m=re.search("[?&]mb=([0-9]{1,3})",self.path);n=int(m.group(1))*1000000 if m else 0
            with lock:
                n=min(n,st["down"]);st["down"]-=n
            if n<=0:return self.reply(429)
            self.send_response(200);self.send_header("Content-Type","application/octet-stream");self.send_header("Content-Length",str(n))
            self.send_header("Cache-Control","no-store");self.end_headers()
            while n>0:
                k=min(n,len(BLOB));self.wfile.write(BLOB[:k]);n-=k
            return
        self.reply(404)
    def do_POST(self):
        p=self.part()
        try:n=int(self.headers.get("Content-Length") or 0)
        except ValueError:n=-1
        if p is None or n<0 or n>4198400:self.close_connection=True;return self.reply(404)
        if p=="up":
            with lock:
                ok=st["up"]>=n;st["up"]-=n if ok else 0
            left=n
            while left>0:
                b=self.rfile.read(min(left,65536))
                if not b:break
                left-=len(b)
            return self.reply(200,b"ok") if ok else self.reply(429)
        if p=="done" and n<=8192:
            try:
                j=json.loads(self.rfile.read(n).decode("utf-8"))
                pg=[fnum(x,60000) for x in j.get("pings",[])][:40]
                w=lambda k:j.get(k) if isinstance(j.get(k),str) and re.match("^[a-z0-9./-]{1,16}$",j.get(k)) else "-"
                out("direct pings="+",".join("%.2f"%x for x in pg)+" lost=%d proto=%s conn=%s"%(int(fnum(j.get("lost",0),100)),w("proto"),w("conn")))
                for k in ("down","up"):
                    t=j.get(k) or {}
                    out("direct %s bytes=%d ms=%.0f conns=%d"%(k,int(fnum(t.get("bytes",0),1e12)),fnum(t.get("ms",0),600000),int(fnum(t.get("conns",1),64))))
            except Exception:
                return self.reply(400)
            st["done"]=True
            return self.reply(200,b"ok")
        self.reply(404)
class S(socketserver.ThreadingTCPServer):
    daemon_threads=True;allow_reuse_address=True
class S6(S):
    address_family=socket.AF_INET6
    def server_bind(self):
        self.socket.setsockopt(socket.IPPROTO_IPV6,socket.IPV6_V6ONLY,0)
        S.server_bind(self)
try:srv=S6(("::",PORT),H)
except Exception:
    try:srv=S(("0.0.0.0",PORT),H)
    except Exception:
        out("error=port-in-use");sys.exit(0)
threading.Thread(target=srv.serve_forever,daemon=True).start()
out("listen port=%d"%PORT)
t0=time.time()
while not st["done"]:
    time.sleep(0.2)
    if st["ip"] is None and time.time()-t0>__WAIT__:
        out("error=no-visitor");break
    if st["ip"] is not None and time.time()-st["first"]>__RUN__:
        out("error=not-finished");break
time.sleep(0.3)
`

// DirectCommand 生成直连测速的命令：在主机上开临时测速服务，等浏览器测完，再从主机追踪到它看到的来源地址并 ping 十次。
// token 是面板生成的一次性口令（32 位十六进制）；install 为真时先装上缺少的 traceroute。
func DirectCommand(port int, token string, sizeMB int, install bool) (string, error) {
	if !ValidDirectPort(port) {
		return "", errors.New("端口应该在 1024 到 65535 之间")
	}
	if !directTokenRe.MatchString(token) {
		return "", errors.New("口令不对")
	}
	if !validClientSize(sizeMB) {
		return "", errors.New("数据量不对")
	}
	script := strings.NewReplacer("__WAIT__", strconv.Itoa(DirectWait), "__RUN__", strconv.Itoa(DirectRun)).Replace(directScript)
	p, mb := strconv.Itoa(port), strconv.Itoa(sizeMB)
	var b strings.Builder
	if install {
		b.WriteString(installTraceroute)
	}
	b.WriteString(directMark + p + " size=" + mb + "'; ")
	b.WriteString("if ! command -v python3 >/dev/null 2>&1; then echo 'speedtest error=no-python3'; else python3 -c '" + script + "' " + p + " " + token + " " + mb + " 2>&1 | { ip=''; " +
		"while IFS= read -r l; do echo \"$l\"; case \"$l\" in 'speedtest client ip='*) ip=\"${l#speedtest client ip=}\";; esac; done; " +
		"if [ -n \"$ip\" ]; then " + traceLoop("\"$ip\"") + backPing("\"$ip\"") + "; fi; }; fi")
	return b.String(), nil
}

var directArgsRe = regexp.MustCompile(`' (\d{4,5}) ([0-9a-f]{32}) (\d{1,3}) 2>&1 \| \{ ip='';`)

// directSpec 认出 DirectCommand 生成的命令：读出端口、口令、数据量后重新生成一遍，逐字相同才算。
func directSpec(command string) (Spec, bool) {
	m := directArgsRe.FindStringSubmatch(command)
	if m == nil {
		return Spec{}, false
	}
	port, _ := strconv.Atoi(m[1])
	size, _ := strconv.Atoi(m[3])
	for _, install := range []bool{false, true} {
		if c, err := DirectCommand(port, m[2], size, install); err == nil && c == command {
			return Spec{Method: MethodDirect, SizeMB: size}, true
		}
	}
	return Spec{}, false
}

// DirectInfo 从直连测速的命令里读出端口和口令，给发起人拼出测速页的地址；不是这种命令时 ok 为 false。
func DirectInfo(command string) (port int, token string, ok bool) {
	if _, is := directSpec(command); !is {
		return 0, "", false
	}
	m := directArgsRe.FindStringSubmatch(command)
	port, _ = strconv.Atoi(m[1])
	return port, m[2], true
}

// addDirectLine 把一行 "speedtest direct …" 并进 c（临时测速服务转述的浏览器测量结果）。
func addDirectLine(c *Client, rest string) *Client {
	m := kv(rest)
	if c == nil {
		c = &Client{V: ClientVersion, Pings: []float64{}}
	}
	transfer := func() *Transfer {
		bytes, ms := num(m, "bytes"), num(m, "ms")
		if bytes <= 0 || ms <= 0 {
			return nil
		}
		return &Transfer{Bytes: bytes, MS: round(ms), Bps: round(bytes * 8 / (ms / 1000)), Conns: int(num(m, "conns"))}
	}
	switch {
	case strings.HasPrefix(rest, "pings="):
		for _, f := range strings.Split(m["pings"], ",") {
			if v, err := strconv.ParseFloat(f, 64); err == nil && len(c.Pings) < 40 {
				c.Pings = append(c.Pings, v)
			}
		}
		c.Lost = int(num(m, "lost"))
		if v := m["proto"]; v != "-" {
			c.Proto = v
		}
		if v := m["conn"]; v != "-" {
			c.Conn = v
		}
	case strings.HasPrefix(rest, "down "):
		c.Down = transfer()
	case strings.HasPrefix(rest, "up "):
		c.Up = transfer()
	}
	return c
}

// directIP 校验临时测速服务报的来源地址。
func directIP(s string) string {
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" || a.IsUnspecified() {
		return ""
	}
	return a.Unmap().String()
}
