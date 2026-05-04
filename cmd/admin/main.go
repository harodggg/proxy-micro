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

var (
	startTime  = time.Now()
	totalReqs  atomic.Int64
	activeReqs atomic.Int64
	bytesIn    atomic.Int64
	bytesOut   atomic.Int64
)

type AdminStats struct {
	Uptime    string `json:"uptime"`
	Version   string `json:"version"`
	Requests  int64  `json:"total_requests"`
	Active    int64  `json:"active_requests"`
	BytesIn   int64  `json:"bytes_in"`
	BytesOut  int64  `json:"bytes_out"`
	Services  []ServiceInfo `json:"services"`
}

type ServiceInfo struct {
	Name     string `json:"name"`
	Bind     string `json:"bind"`
	Status   string `json:"status"`
}

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Proxy Micro · Admin</title>
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
         background: #0d1117; color: #c9d1d9; min-height: 100vh; }
  .nav { background: #161b22; border-bottom:1px solid #30363d; padding:1rem 2rem;
         display:flex; align-items:center; gap:1rem; }
  .nav h1 { font-size:1.3rem; font-weight:600; }
  .nav h1 span { color:#58a6ff; }
  .badge { background:#21262d; padding:0.25rem 0.75rem; border-radius:999px;
           font-size:0.8rem; color:#8b949e; }
  .container { max-width: 1000px; margin:0 auto; padding:2rem; }
  .grid { display:grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
          gap:1rem; margin-bottom:2rem; }
  .card { background:#161b22; border:1px solid #30363d; border-radius:8px;
          padding:1.5rem; }
  .card .label { font-size:0.8rem; color:#8b949e; text-transform:uppercase;
                 letter-spacing:0.05em; margin-bottom:0.5rem; }
  .card .value { font-size:2rem; font-weight:700; color:#f0f6fc; }
  .card .value.green { color:#3fb950; }
  .card .value.blue { color:#58a6ff; }
  .card .value.orange { color:#d29922; }
  .card .value.pink { color:#f778ba; }
  table { width:100%; border-collapse:collapse; background:#161b22;
          border:1px solid #30363d; border-radius:8px; overflow:hidden; }
  th, td { padding:0.75rem 1rem; text-align:left; border-bottom:1px solid #21262d; }
  th { font-size:0.8rem; color:#8b949e; text-transform:uppercase; background:#0d1117; }
  td.status-online { color:#3fb950; }
  td.status-offline { color:#f85149; }
  .footer { text-align:center; padding:2rem; color:#484f58; font-size:0.85rem; }
  @media(max-width:600px) { .nav { padding:1rem; } .container { padding:1rem; } }
</style>
</head>
<body>
  <div class="nav">
    <h1>⚡ <span>Proxy</span>Micro</h1>
    <span class="badge">v1.0.0</span>
    <span class="badge" id="uptime">--</span>
  </div>
  <div class="container">
    <div class="grid">
      <div class="card">
        <div class="label">总请求数</div>
        <div class="value green" id="totalReqs">0</div>
      </div>
      <div class="card">
        <div class="label">当前活跃连接</div>
        <div class="value blue" id="activeReqs">0</div>
      </div>
      <div class="card">
        <div class="label">总入站流量</div>
        <div class="value orange" id="bytesIn">0 B</div>
      </div>
      <div class="card">
        <div class="label">总出站流量</div>
        <div class="value pink" id="bytesOut">0 B</div>
      </div>
    </div>
    <h2 style="font-size:1.1rem;margin-bottom:1rem;color:#8b949e;">服务状态</h2>
    <table>
      <thead><tr><th>服务</th><th>监听地址</th><th>状态</th></tr></thead>
      <tbody id="services-tbody"></tbody>
    </table>
  </div>
  <div class="footer">Proxy Micro · 微服务代理工具</div>
  <script>
  function formatBytes(b) { if(b===0) return '0 B';
    const u=['B','KB','MB','GB','TB']; let i=0;
    while(b>=1024&&i<u.length-1){b/=1024;i++}
    return b.toFixed(1)+' '+u[i]; }

  function fetchStats() {
    fetch('/api/stats').then(r=>r.json()).then(d=>{
      document.getElementById('uptime').textContent = '⏱ '+d.uptime;
      document.getElementById('totalReqs').textContent = d.total_requests;
      document.getElementById('activeReqs').textContent = d.active_requests;
      document.getElementById('bytesIn').textContent = formatBytes(d.bytes_in);
      document.getElementById('bytesOut').textContent = formatBytes(d.bytes_out);
      const tbody = document.getElementById('services-tbody');
      tbody.innerHTML = d.services.map(s=>
        '<tr><td>'+s.name+'</td><td>'+s.bind+'</td>'+
        '<td class="status-'+s.status+'">● '+s.status+'</td></tr>'
      ).join('');
    }).catch(()=>{});
  }
  fetchStats();
  setInterval(fetchStats, 3000);
  </script>
</body>
</html>`

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

	// 解析模板
	tmpl := template.Must(template.New("admin").Parse(indexHTML))

	mux := http.NewServeMux()

	// 管理界面
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, nil)
	})

	// API: 统计信息
	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		uptime := time.Since(startTime).Round(time.Second).String()
		stats := AdminStats{
			Uptime:   uptime,
			Version:  "1.0.0",
			Requests: totalReqs.Load(),
			Active:   activeReqs.Load(),
			BytesIn:  bytesIn.Load(),
			BytesOut: bytesOut.Load(),
			Services: []ServiceInfo{
				{Name: "HTTP Proxy",  Bind: cfg.Services.HTTPProxy.Bind,  Status: boolStatus(cfg.Services.HTTPProxy.Enabled)},
				{Name: "SOCKS5 Proxy", Bind: cfg.Services.SOCKS5Proxy.Bind, Status: boolStatus(cfg.Services.SOCKS5Proxy.Enabled)},
				{Name: "Admin API",   Bind: cfg.Services.Admin.Bind,      Status: "online"},
			},
		}
		json.NewEncoder(w).Encode(stats)
	})

	// API: 健康检查
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	listenAddr := cfg.Services.Admin.Bind
	if listenAddr == "" {
		listenAddr = ":8088"
	}

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	fmt.Printf("📊 [Admin] dashboard: http://%s\n", listenAddr)

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

func boolStatus(enabled bool) string {
	if enabled {
		return "online"
	}
	return "offline"
}
