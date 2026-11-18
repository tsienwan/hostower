package speedtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGeo(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var ips []string
		json.NewDecoder(r.Body).Decode(&ips)
		if r.URL.Query().Get("lang") != "zh-CN" {
			t.Errorf("语言：%q", r.URL.RawQuery)
		}
		var out []map[string]string
		for _, ip := range ips {
			out = append(out, map[string]string{"status": "success", "query": ip, "country": "示例国\x07", "city": "示例市", "isp": "示例网络", "as": "AS4809 Example"})
		}
		out = append(out, map[string]string{"status": "success", "query": "203.0.113.99", "country": "不该出现"})
		json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	s := &Service{geo: newGeoCache()}
	s.geo.url = srv.URL + "/?lang="

	ips := []string{"198.51.100.1", "10.0.0.1", "198.51.100.1", "", "100.64.0.1"} // 内网和运营商级 NAT 地址：不该发出去 hygiene:allow
	got, err := s.Geo(context.Background(), ips, "zh-CN")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["198.51.100.1"].Country != "示例国" || got["198.51.100.1"].Net != "CN2" {
		t.Fatalf("结果不对：%+v", got)
	}
	// 第二次走缓存，不再请求。
	if _, err := s.Geo(context.Background(), ips, "zh-CN"); err != nil || calls != 1 {
		t.Fatalf("应该用缓存：%d 次请求，%v", calls, err)
	}

	routes := []Route{{Target: "198.51.100.9", Hops: []Hop{{N: 1, IP: "10.0.0.1", Net: "LAN"}, {N: 2, IP: "198.51.100.1"}, {N: 3}}, Nets: []string{}}} // hygiene:allow
	re := Relabel(routes, got)
	if re[0].Hops[1].Net != "CN2" || re[0].Line != "电信 CN2 GIA" || routes[0].Hops[1].Net != "" {
		t.Fatalf("重新推测不对：%+v", re[0])
	}
}

func TestRouteICMP(t *testing.T) {
	// busybox traceroute -I 的输出
	r := Parse("speedtest route speed.example.com traceroute to speed.example.com (192.0.2.8), 30 hops max, 46 byte packets\n" +
		"speedtest route speed.example.com  1  172.17.0.1  0.004 ms\n" + // hygiene:allow
		"speedtest route speed.example.com  2  192.0.2.8  70.666 ms\n").Detail.Routes
	if len(r) != 1 || len(r[0].Hops) != 2 || r[0].Hops[0].IP != "172.17.0.1" || r[0].Hops[1].MS != 70.67 { // hygiene:allow
		t.Fatalf("%+v", r)
	}
}

// 地址段认成 CMI、自治系统号是精品网时，改标成 CMIN2；其余已认出的不被自治系统号改掉。
func TestRelabelRefinesCMI(t *testing.T) {
	routes := []Route{{Target: "198.51.100.9", Hops: []Hop{{N: 1, IP: "198.51.100.1", Net: "CMI"}, {N: 2, IP: "198.51.100.2", Net: "CMNET"}}, Nets: []string{}}} // hygiene:allow
	geo := map[string]GeoInfo{"198.51.100.1": {Net: "CMIN2"}, "198.51.100.2": {Net: "CMIN2"}}                                                                    // hygiene:allow
	re := Relabel(routes, geo)
	if re[0].Hops[0].Net != "CMIN2" || re[0].Hops[1].Net != "CMNET" || re[0].Line != "移动 CMIN2" {
		t.Fatalf("没有改标成 CMIN2：%+v", re[0])
	}
}
