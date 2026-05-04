package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/proxy-micro/pkg/config"
)

var startTime = time.Now()

// 从代理服务轮询的聚合统计
var (
	polledTotalConns  atomic.Int64
	polledActiveConns atomic.Int64
	polledBytesIn     atomic.Int64
	polledBytesOut    atomic.Int64
)

type ProxyStatsSnapshot struct {
	TotalConns  int64 `json:"total_connections"`
	ActiveConns int64 `json:"active_connections"`
	BytesIn     int64 `json:"bytes_in"`
	BytesOut    int64 `json:"bytes_out"`
}

type AdminStats struct {
	Uptime    string        `json:"uptime"`
	Version   string        `json:"version"`
	Requests  int64         `json:"total_requests"`
	Active    int64         `json:"active_requests"`
	BytesIn   int64         `json:"bytes_in"`
	BytesOut  int64         `json:"bytes_out"`
	Services  []ServiceInfo `json:"services"`
}

type ServiceInfo struct {
	Name   string `json:"name"`
	Bind   string `json:"bind"`
	Status string `json:"status"`
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Proxy Micro · 代理管理面板</title>
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', system-ui, sans-serif;
         background: #0d1117; color: #c9d1d9; min-height: 100vh; }
  .nav { background: #161b22; border-bottom:1px solid #30363d; padding:1rem 2rem;
         display:flex; align-items:center; gap:1rem; flex-wrap:wrap; }
  .nav h1 { font-size:1.3rem; font-weight:600; }
  .nav h1 span { color:#58a6ff; }
  .badge { background:#21262d; padding:0.25rem 0.75rem; border-radius:999px;
           font-size:0.8rem; color:#8b949e; }
  .nav-right { margin-left:auto; display:flex; gap:0.75rem; align-items:center; }
  .container { max-width: 1100px; margin:0 auto; padding:2rem; }
  .grid { display:grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
          gap:1rem; margin-bottom:2rem; }
  .card { background:#161b22; border:1px solid #30363d; border-radius:8px;
          padding:1.5rem; transition: border-color 0.2s; }
  .card:hover { border-color:#58a6ff40; }
  .card .label { font-size:0.75rem; color:#8b949e; text-transform:uppercase;
                 letter-spacing:0.05em; margin-bottom:0.5rem; }
  .card .value { font-size:2rem; font-weight:700; }
  .card .value.green { color:#3fb950; }
  .card .value.blue { color:#58a6ff; }
  .card .value.orange { color:#d29922; }
  .card .value.pink { color:#f778ba; }
  table { width:100%; border-collapse:collapse; background:#161b22;
          border:1px solid #30363d; border-radius:8px; overflow:hidden; margin-bottom:2rem; }
  th { font-size:0.75rem; color:#8b949e; text-transform:uppercase; background:#0d1117; }
  th, td { padding:0.75rem 1rem; text-align:left; border-bottom:1px solid #21262d; }
  .status-online { color:#3fb950; }
  .status-offline { color:#f85149; }
  .mono { font-family: 'SF Mono', 'Fira Code', monospace; font-size:0.9rem; }
  .test-box { background:#161b22; border:1px solid #30363d; border-radius:8px; padding:1.5rem; margin-bottom:2rem; }
  .test-box h3 { font-size:1rem; margin-bottom:1rem; }
  .test-box code { display:block; background:#0d1117; padding:1rem; border-radius:6px;
                   font-size:0.85rem; line-height:1.6; margin-bottom:0.5rem;
                   border:1px solid #21262d; word-break:break-all; }
  .test-box code .comment { color:#8b949e; }
  .test-box code .cmd { color:#f778ba; }
  .test-box code .url { color:#58a6ff; }
  .footer { text-align:center; padding:2rem; color:#484f58; font-size:0.85rem; }
  .status-dot { display:inline-block; width:8px; height:8px; border-radius:50%; margin-right:0.4rem; }
  @media(max-width:600px) { .nav { padding:1rem; } .container { padding:1rem; }
    .grid { grid-template-columns:1fr 1fr; } }
</style>
</head>
<body>
  <div class="nav">
    <h1>⚡ <span>Proxy</span>Micro</h1>
    <span class="badge" id="version">v1.0.0</span>
    <span class="badge" id="uptimeBadge">⏱ --</span>
    <div class="nav-right">
      <span class="badge" style="background:#3fb95020;color:#3fb950;" id="statusBadge">● 运行中</span>
    </div>
  </div>
  <div class="container">
    <div class="grid">
      <div class="card">
        <div class="label">总连接数</div>
        <div class="value green" id="totalReqs">0</div>
      </div>
      <div class="card">
        <div class="label">活跃连接</div>
        <div class="value blue" id="activeReqs">0</div>
      </div>
      <div class="card">
        <div class="label">入站流量</div>
        <div class="value orange" id="bytesIn">0 B</div>
      </div>
      <div class="card">
        <div class="label">出站流量</div>
        <div class="value pink" id="bytesOut">0 B</div>
      </div>
      <div class="card">
        <div class="label">运行时长</div>
        <div class="value" style="font-size:1.4rem;color:#f0f6fc" id="uptimeValue">--</div>
      </div>
    </div>
    <h2 style="font-size:1rem;margin-bottom:0.75rem;color:#8b949e;">📡 服务状态</h2>
    <table>
      <thead><tr><th>服务</th><th>协议</th><th>监听地址</th><th>状态</th><th>连接</th><th>入站</th><th>出站</th><th>用法</th></tr></thead>
      <tbody id="servicesBody"></tbody>
    </table>
    <h2 style="font-size:1rem;margin-bottom:0.75rem;color:#8b949e;">🔌 连接测试</h2>
    <div class="test-box">
      <h3>HTTP 代理</h3>
      <code><span class="comment"># 终端执行</span>
<span class="cmd">curl</span> -x <span class="url">http://{{.ServerIP}}:8080</span> https://httpbin.org/ip</code>
    </div>
    <div class="test-box">
      <h3>SOCKS5 代理</h3>
      <code><span class="comment"># 终端执行</span>
<span class="cmd">curl</span> --socks5-hostname <span class="url">{{.ServerIP}}:1080</span> https://httpbin.org/ip</code>
    </div>
  </div>
  <div class="footer">Proxy Micro · <a style="color:#58a6ff;" href="https://github.com/harodggg/proxy-micro">GitHub</a></div>

  <script>
  function formatBytes(b) {
    if(b===0) return '0 B';
    const u=['B','KB','MB','GB','TB']; let i=0;
    while(b>=1024&&i<4){b/=1024;i++}
    return b.toFixed(1)+' '+u[i];
  }
  function formatDuration(s) {
    const d=Math.floor(s/86400); s-=d*86400;
    const h=Math.floor(s/3600); s-=h*3600;
    const m=Math.floor(s/60); s-=m*60;
    return (d>0?d+'天 ':'')+String(h).padStart(2,'0')+':'+String(m).padStart(2,'0')+':'+String(s).padStart(2,'0');
  }

  const services = [
    { name:'HTTP Proxy',  proto:'HTTP/HTTPS', bind:':8080', test:'curl -x %s:8080 https://httpbin.org/ip' },
    { name:'SOCKS5 Proxy', proto:'SOCKS5',     bind:':1080', test:'curl --socks5-hostname %s:1080 https://httpbin.org/ip' },
    { name:'Admin Dashboard', proto:'HTTP',    bind:':8088', test:'open http://%s:8088' }
  ];

  function fetchStats() {
    fetch('/api/stats').then(r=>r.json()).then(d=>{
      document.getElementById('uptimeBadge').textContent = '⏱ '+d.uptime;
      document.getElementById('uptimeValue').textContent = d.uptime;
      document.getElementById('totalReqs').textContent = d.total_requests;
      document.getElementById('activeReqs').textContent = d.active_requests;
      document.getElementById('bytesIn').textContent = formatBytes(d.bytes_in);
      document.getElementById('bytesOut').textContent = formatBytes(d.bytes_out);
      const tbody = document.getElementById('servicesBody');
      const serverIP = d.server_ip || '{{.ServerIP}}';
      tbody.innerHTML = services.map(s => {
        const svc = d.services.find(x => x.name === s.name);
        const status = svc ? svc.status : 'offline';
        const cls = status === 'online' ? 'status-online' : 'status-offline';
        const conns = svc ? (svc.connections||'--') : '--';
        const bin = svc ? formatBytes(svc.bytes_in||0) : '--';
        const bout = svc ? formatBytes(svc.bytes_out||0) : '--';
        return '<tr>' +
          '<td><span class="status-dot" style="background:'+(status==='online'?'#3fb950':'#f85149')+'"></span>'+s.name+'</td>' +
          '<td class="mono">'+s.proto+'</td>' +
          '<td class="mono">'+s.bind+'</td>' +
          '<td class="'+cls+'">● '+status+'</td>' +
          '<td>'+conns+'</td>' +
          '<td>'+bin+'</td>' +
          '<td>'+bout+'</td>' +
          '<td style="font-family:monospace;font-size:0.75rem;color:#8b949e;">'+s.test.replace(/%s/g,serverIP)+'</td>' +
          '</tr>';
      }).join('');
    }).catch(()=>{});
  }
  fetchStats();
  setInterval(fetchStats, 3000);
  </script>
</body>
</html>`

type TemplateData struct {
	ServerIP string
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if !cfg.Services.Admin.Enabled {
		log.Println("Admin service is disabled")
		return
	}

	// 获取服务器 IP
	serverIP := cfg.Services.Admin.Advertise
	if serverIP == "" {
		serverIP = getPublicIP()
	}

	// 定时从代理服务拉取统计数据
	go pollProxyStats()

	tmpl := template.Must(template.New("admin").Parse(indexHTML))
	tmplData := TemplateData{ServerIP: serverIP}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, tmplData)
	})

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		uptime := time.Since(startTime).Round(time.Second).String()

		// 获取各代理的详细统计
		httpStats := fetchProxyStats("http://127.0.0.1:8089/stats")
		socksStats := fetchProxyStats("http://127.0.0.1:1089/stats")

		httpSvc := ServiceInfo{
			Name: "HTTP Proxy", Bind: ":8080",
			Status: boolStatus(cfg.Services.HTTPProxy.Enabled && httpStats != nil),
		}
		socksSvc := ServiceInfo{
			Name: "SOCKS5 Proxy", Bind: ":1080",
			Status: boolStatus(cfg.Services.SOCKS5Proxy.Enabled && socksStats != nil),
		}
		adminSvc := ServiceInfo{
			Name: "Admin Dashboard", Bind: ":8088", Status: "online",
		}

		raw := map[string]interface{}{
			"uptime":          uptime,
			"version":         "1.0.0",
			"total_requests":  polledTotalConns.Load(),
			"active_requests": polledActiveConns.Load(),
			"bytes_in":        polledBytesIn.Load(),
			"bytes_out":       polledBytesOut.Load(),
			"server_ip":       serverIP,
			"services": []map[string]interface{}{
				{
					"name":        httpSvc.Name,
					"bind":        httpSvc.Bind,
					"status":      httpSvc.Status,
					"connections": iface(httpStats, httpStats.TotalConns),
					"bytes_in":    iface(httpStats, httpStats.BytesIn),
					"bytes_out":   iface(httpStats, httpStats.BytesOut),
				},
				{
					"name":        socksSvc.Name,
					"bind":        socksSvc.Bind,
					"status":      socksSvc.Status,
					"connections": iface(socksStats, socksStats.TotalConns),
					"bytes_in":    iface(socksStats, socksStats.BytesIn),
					"bytes_out":   iface(socksStats, socksStats.BytesOut),
				},
				{
					"name":   adminSvc.Name,
					"bind":   adminSvc.Bind,
					"status": adminSvc.Status,
				},
			},
		}

		json.NewEncoder(w).Encode(raw)
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	listenAddr := cfg.Services.Admin.Bind
	if listenAddr == "" {
		listenAddr = ":8088"
	}

	server := &http.Server{Addr: listenAddr, Handler: mux}

	fmt.Printf("📊 [Admin] UI: http://%s:8088\n", serverIP)
	fmt.Printf("   🔗 HTTP Proxy:  http://%s:8080\n", serverIP)
	fmt.Printf("   🔗 SOCKS5:      socks5://%s:1080\n", serverIP)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		fmt.Println("\n⏹  Admin shutting down...")
		server.Close()
	}()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("admin listen: %v", err)
	}
}

func boolStatus(ok bool) string {
	if ok {
		return "online"
	}
	return "offline"
}

func getPublicIP() string {
	resp, err := http.Get("https://api.ipify.org?format=text")
	if err != nil {
		return "localhost"
	}
	defer resp.Body.Close()
	var ip string
	fmt.Fscan(resp.Body, &ip)
	if ip == "" {
		return "localhost"
	}
	return ip
}

// fetchProxyStats 从代理服务拉取统计
func fetchProxyStats(url string) *ProxyStatsSnapshot {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var s ProxyStatsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil
	}
	return &s
}

// pollProxyStats 定时轮询所有代理服务统计
func pollProxyStats() {
	for {
		time.Sleep(2 * time.Second)

		var totalConns, activeConns, bytesIn, bytesOut int64

		for _, url := range []string{
			"http://127.0.0.1:8089/stats",
			"http://127.0.0.1:1089/stats",
		} {
			if s := fetchProxyStats(url); s != nil {
				totalConns += s.TotalConns
				activeConns += s.ActiveConns
				bytesIn += s.BytesIn
				bytesOut += s.BytesOut
			}
		}

		polledTotalConns.Store(totalConns)
		polledActiveConns.Store(activeConns)
		polledBytesIn.Store(bytesIn)
		polledBytesOut.Store(bytesOut)
	}
}

// iface 辅助：如果 stats 为 nil 则返回 "--"
func iface(s *ProxyStatsSnapshot, v int64) interface{} {
	if s == nil {
		return "--"
	}
	return v
}
