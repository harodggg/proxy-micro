package main

import (
	"encoding/json"
	"fmt"
	"text/template"
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
<title>Proxy Micro · 实时流量监控</title>
<style>
  * { margin:0; padding:0; box-sizing:border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', 'PingFang SC', system-ui, sans-serif;
         background: #0a0e14; color: #e6e6f0; }

  /* Layout */
  .nav { background: linear-gradient(135deg,#111822,#162132);
         border-bottom:1px solid #1a2a3a; padding:0.8rem 2rem;
         display:flex; align-items:center; gap:1rem; flex-wrap:wrap; }
  .nav h1 { font-size:1.3rem; font-weight:600; }
  .nav h1 span { color:#58a6ff; }
  .badge { background:#1a2a3a; padding:0.25rem 0.75rem; border-radius:999px;
           font-size:0.75rem; color:#8b949e; }
  .nav-right { margin-left:auto; display:flex; gap:0.75rem; align-items:center; }
  .container { max-width: 1200px; margin:0 auto; padding:1.5rem; }

  /* Speed banner */
  .speed-banner {
    text-align:center; padding:2rem 1rem;
    background: linear-gradient(135deg, #0d1a2b 0%, #1a0d2b 100%);
    border: 1px solid rgba(88,166,255,0.15);
    border-radius:16px; margin-bottom:1.5rem;
    position:relative; overflow:hidden;
  }
  .speed-banner::before {
    content:''; position:absolute; top:-50%; left:-50%; width:200%; height:200%;
    background: conic-gradient(from 0deg, transparent, rgba(88,166,255,0.05), transparent, rgba(247,120,186,0.05), transparent);
    animation: rotateGlow 4s linear infinite;
  }
  @keyframes rotateGlow { to { transform:rotate(360deg); } }
  .speed-banner .label { font-size:0.75rem; letter-spacing:0.15em; color:#586b7a; text-transform:uppercase; }
  .speed-banner .speed-value {
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: clamp(2.5rem, 6vw, 5rem); font-weight:800;
    background: linear-gradient(135deg, #58a6ff, #f778ba, #fdcb6e);
    -webkit-background-clip: text; -webkit-text-fill-color: transparent;
    background-clip: text; line-height:1.2;
  }
  .speed-banner .speed-detail { display:flex; justify-content:center; gap:2rem; margin-top:0.5rem; }
  .speed-banner .speed-detail span { font-size:0.85rem; color:#8b949e; }
  .speed-banner .speed-detail b { color:#58a6ff; font-family:monospace; }

  /* Stats grid */
  .grid { display:grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
          gap:0.75rem; margin-bottom:1.5rem; }
  .card { background:#111822; border:1px solid #1a2a3a; border-radius:12px;
          padding:1.2rem; transition: all 0.3s; position:relative; overflow:hidden; }
  .card.dl { border-color:rgba(88,166,255,0.2); }
  .card.ul { border-color:rgba(247,120,186,0.2); }
  .card .label { font-size:0.65rem; color:#586b7a; text-transform:uppercase; letter-spacing:0.06em; margin-bottom:0.3rem; }
  .card .value { font-size:1.5rem; font-weight:700; transition: all 0.2s; }
  .card .value.green { color:#3fb950; }
  .card .value.blue { color:#58a6ff; }
  .card .value.pink { color:#f778ba; }
  .card .value.gold { color:#fdcb6e; }
  .card .sub-label { font-size:0.7rem; color:#586b7a; margin-top:0.2rem; }

  /* Flow bar animation */
  .flow-bar { height:2px; margin-top:0.5rem; border-radius:2px;
              background:#1a2a3a; overflow:hidden; }
  .flow-bar .fill { height:100%; border-radius:2px; transition:width 0.3s;
                    background: linear-gradient(90deg, #58a6ff, #79c0ff); }
  .flow-bar .fill.pink { background: linear-gradient(90deg, #f778ba, #ff99cc); }
  .flow-bar .fill.green { background: linear-gradient(90deg, #3fb950, #56d364); }

  /* Sparkline */
  .sparkline-wrap { background:#111822; border:1px solid #1a2a3a; border-radius:12px;
                    padding:1rem; margin-bottom:1.5rem; }
  .sparkline-wrap .spark-header { display:flex; justify-content:space-between; margin-bottom:0.5rem; font-size:0.75rem; }
  .sparkline-wrap .spark-header span { color:#8b949e; }
  .sparkline-wrap .spark-header b { color:#58a6ff; font-family:monospace; }
  #sparkline { width:100%; height:80px; }

  /* Table */
  table { width:100%; border-collapse:collapse; background:#111822;
          border:1px solid #1a2a3a; border-radius:12px; overflow:hidden; margin-bottom:1.5rem; }
  th { font-size:0.65rem; color:#586b7a; text-transform:uppercase; background:#0a0e14; }
  th, td { padding:0.6rem 0.8rem; text-align:left; border-bottom:1px solid #1a2a3a; font-size:0.85rem; }
  .status-online { color:#3fb950; }
  .status-offline { color:#f85149; }
  .mono { font-family: 'SF Mono', 'Fira Code', monospace; }

  .footer { text-align:center; padding:2rem; color:#586b7a; font-size:0.8rem; }

  /* Pulse animation for active connections */
  @keyframes pulse { 0%,100% { opacity:1; } 50% { opacity:0.3; } }
  .pulsing { animation: pulse 1s infinite; }

  @media(max-width:600px) { .nav { padding:0.8rem 1rem; } .container { padding:1rem; }
    .grid { grid-template-columns:1fr 1fr; } .speed-banner .speed-detail { gap:1rem; flex-wrap:wrap; } }
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
    <!-- Speed banner -->
    <div class="speed-banner">
      <div class="label">当前带宽</div>
      <div class="speed-value" id="currentSpeed">0 MB/s</div>
      <div class="speed-detail">
        <span>⬇ 入站 <b id="speedIn">0 B/s</b></span>
        <span>⬆ 出站 <b id="speedOut">0 B/s</b></span>
      </div>
    </div>

    <!-- Stats cards -->
    <div class="grid">
      <div class="card">
        <div class="label">📊 总流量</div>
        <div class="value blue" id="totalTraffic">0 B</div>
        <div class="sub-label" id="totalBreakdown">⬇ 0 B / ⬆ 0 B</div>
      </div>
      <div class="card dl">
        <div class="label">⬇ 入站累计</div>
        <div class="value blue" id="bytesIn">0 B</div>
        <div class="flow-bar"><div class="fill" id="flowIn" style="width:0%"></div></div>
      </div>
      <div class="card ul">
        <div class="label">⬆ 出站累计</div>
        <div class="value pink" id="bytesOut">0 B</div>
        <div class="flow-bar"><div class="fill pink" id="flowOut" style="width:0%"></div></div>
      </div>
      <div class="card">
        <div class="label">📈 连接</div>
        <div class="value green" id="totalReqs">0</div>
        <div class="sub-label">活跃: <b id="activeReqs" style="color:#3fb950">0</b></div>
      </div>
    </div>

    <!-- Sparkline -->
    <div class="sparkline-wrap">
      <div class="spark-header">
        <span>📉 实时带宽曲线（最近 60s）</span>
        <span>峰值: <b id="peakSpeed">0 MB/s</b></span>
      </div>
      <canvas id="sparkline"></canvas>
    </div>

    <!-- Services table -->
    <h2 style="font-size:0.85rem;margin-bottom:0.6rem;color:#8b949e;">📡 服务详情</h2>
    <table>
      <thead><tr><th>服务</th><th>协议</th><th>监听</th><th>状态</th><th>连接</th><th>入站</th><th>出站</th></tr></thead>
      <tbody id="servicesBody"></tbody>
    </table>
  </div>

  <div class="footer">
    Proxy Micro · <a style="color:#58a6ff;" href="https://github.com/harodggg/proxy-micro">GitHub</a>
    &nbsp;·&nbsp; curl -x http://{{.ServerIP}}:8080 https://httpbin.org/ip
  </div>

  <script>
  function formatBytes(b) {
    if(!b||b===0) return '0 B';
    const u=['B','KB','MB','GB','TB']; let i=0;
    const n=Math.abs(b);
    while(n>=1024&&i<4){b/=1024;i++}
    return b.toFixed(i<2?0:1)+' '+u[i];
  }
  function formatSpeed(bps) {
    if(!bps||bps<1) return '0 B/s';
    const u=['B/s','KB/s','MB/s','GB/s','TB/s']; let i=0;
    let v=bps;
    while(v>=1024&&i<4){v/=1024;i++}
    return v.toFixed(i<1?0:1)+' '+u[i];
  }

  const services = [
    { name:'HTTP Proxy',  proto:'HTTP', bind:':8080' },
    { name:'SOCKS5 Proxy', proto:'SOCKS5', bind:':1080' },
    { name:'Admin',       proto:'HTTP', bind:':8088' }
  ];

  // Sparkline data
  const MAX_POINTS = 60;
  let bwHistory = [];

  function drawSparkline() {
    const canvas = document.getElementById('sparkline');
    const rect = canvas.parentElement.getBoundingClientRect();
    const dpr = window.devicePixelRatio || 1;
    const w = rect.width - 32;
    const h = 80;
    canvas.width = w * dpr;
    canvas.height = h * dpr;
    canvas.style.width = w + 'px';
    canvas.style.height = h + 'px';
    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    ctx.clearRect(0,0,w,h);
    if(bwHistory.length<2) return;

    const max = Math.max(...bwHistory.map(d=>d.out+d.in), 1);
    const pad = 4;

    // Grid lines
    ctx.strokeStyle = '#1a2a3a';
    ctx.lineWidth = 1;
    for(let y=0;y<h;y+=h/4) {
      ctx.beginPath(); ctx.moveTo(0,y); ctx.lineTo(w,y); ctx.stroke();
    }

    // In traffic (blue fill)
    ctx.beginPath();
    ctx.moveTo(0, h);
    bwHistory.forEach((d,i) => {
      const x = i/(bwHistory.length-1)*(w-pad*2)+pad;
      const v = d.in / max;
      ctx.lineTo(x, h - v*(h-pad*2));
    });
    ctx.lineTo(w, h);
    ctx.closePath();
    const grad1 = ctx.createLinearGradient(0,0,0,h);
    grad1.addColorStop(0,'rgba(88,166,255,0.4)');
    grad1.addColorStop(1,'rgba(88,166,255,0.02)');
    ctx.fillStyle = grad1;
    ctx.fill();

    // Out traffic (pink line)
    ctx.beginPath();
    bwHistory.forEach((d,i) => {
      const x = i/(bwHistory.length-1)*(w-pad*2)+pad;
      const v = d.out / max;
      if(i===0) ctx.moveTo(x, h - v*(h-pad*2));
      else ctx.lineTo(x, h - v*(h-pad*2));
    });
    ctx.strokeStyle = '#f778ba';
    ctx.lineWidth = 2;
    ctx.stroke();

    // In traffic line
    ctx.beginPath();
    bwHistory.forEach((d,i) => {
      const x = i/(bwHistory.length-1)*(w-pad*2)+pad;
      const v = d.in / max;
      if(i===0) ctx.moveTo(x, h - v*(h-pad*2));
      else ctx.lineTo(x, h - v*(h-pad*2));
    });
    ctx.strokeStyle = '#58a6ff';
    ctx.lineWidth = 2;
    ctx.stroke();
  }

  let prevStats = { bytesIn:0, bytesOut:0, ts:Date.now() };
  let animFrame = 0;

  function animateValue(el, target, suffix, decimals) {
    const current = parseFloat(el.getAttribute('data-val')||'0');
    if(Math.abs(target-current)<0.1) {
      el.textContent = target.toFixed(decimals||0)+suffix;
      el.setAttribute('data-val',target);
      return;
    }
    const step = (target-current)*0.3;
    const next = current+step;
    el.textContent = next.toFixed(decimals||0)+suffix;
    el.setAttribute('data-val',next);
  }

  function fetchStats() {
    fetch('/api/stats').then(r=>r.json()).then(d=>{
      const now = Date.now();
      const dt = (now - prevStats.ts) / 1000;

      // Calculate speed
      const dIn = d.bytes_in - prevStats.bytesIn;
      const dOut = d.bytes_out - prevStats.bytesOut;
      const speedIn = dt>0 ? Math.max(0, dIn/dt) : 0;
      const speedOut = dt>0 ? Math.max(0, dOut/dt) : 0;

      document.getElementById('uptimeBadge').textContent = '\u23f1 '+d.uptime;

      // Speed display
      const totalSpeed = speedIn + speedOut;
      document.getElementById('currentSpeed').textContent = formatSpeed(totalSpeed);
      document.getElementById('speedIn').textContent = formatSpeed(speedIn);
      document.getElementById('speedOut').textContent = formatSpeed(speedOut);

      // Sparkline
      bwHistory.push({ in:speedIn, out:speedOut, ts:now });
      if(bwHistory.length>MAX_POINTS) bwHistory.shift();
      const peak = Math.max(...bwHistory.map(d=>d.out+d.in), 0);
      document.getElementById('peakSpeed').textContent = formatSpeed(peak);
      drawSparkline();

      // Cumulative totals
      document.getElementById('bytesIn').textContent = formatBytes(d.bytes_in);
      document.getElementById('bytesOut').textContent = formatBytes(d.bytes_out);

      const total = d.bytes_in + d.bytes_out;
      document.getElementById('totalTraffic').textContent = formatBytes(total);
      document.getElementById('totalBreakdown').innerHTML = '\u2b07 '+formatBytes(d.bytes_in)+' / \u2b06 '+formatBytes(d.bytes_out);

      document.getElementById('totalReqs').textContent = d.total_requests;
      document.getElementById('activeReqs').textContent = d.active_requests;

      // Flow bars (relative to total)
      const maxFlow = Math.max(d.bytes_in,d.bytes_out,1);
      document.getElementById('flowIn').style.width = Math.min(100,(d.bytes_in/maxFlow)*100)+'%';
      document.getElementById('flowOut').style.width = Math.min(100,(d.bytes_out/maxFlow)*100)+'%';

      // Services table
      const tbody = document.getElementById('servicesBody');
      tbody.innerHTML = services.map(s => {
        const svc = d.services.find(x => x.name === s.name);
        const ok = svc && svc.status==='online';
        const cls = ok ? 'status-online' : 'status-offline';
        return '<tr>' +
          '<td><span class="status-dot" style="background:'+(ok?'#3fb950':'#f85149')+'"></span>'+s.name+'</td>' +
          '<td class="mono">'+s.proto+'</td>' +
          '<td class="mono">'+s.bind+'</td>' +
          '<td class="'+cls+'">\u25cf '+(ok?'online':'offline')+'</td>' +
          '<td>'+(svc?svc.connections||0:'--')+'</td>' +
          '<td>'+(svc?formatBytes(svc.bytes_in||0):'--')+'</td>' +
          '<td>'+(svc?formatBytes(svc.bytes_out||0):'--')+'</td>' +
          '</tr>';
      }).join('');

      // Store for speed calc
      prevStats = { bytesIn:d.bytes_in, bytesOut:d.bytes_out, ts:now };
    }).catch(()=>{});
  }

  fetchStats();
  setInterval(fetchStats, 1000);
  window.addEventListener('resize', drawSparkline);
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
		time.Sleep(1 * time.Second)

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
