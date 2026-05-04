package stats

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Tracker 流量跟踪器
type Tracker struct {
	TotalConns  atomic.Int64
	ActiveConns atomic.Int64
	BytesIn     atomic.Int64
	BytesOut    atomic.Int64
}

// Snapshot 流量快照
type Snapshot struct {
	TotalConns  int64 `json:"total_connections"`
	ActiveConns int64 `json:"active_connections"`
	BytesIn     int64 `json:"bytes_in"`
	BytesOut    int64 `json:"bytes_out"`
}

// Handler HTTP handler 返回统计信息
func (t *Tracker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(t.Snapshot())
	})
	return mux
}

// Snapshot 获取当前快照
func (t *Tracker) Snapshot() Snapshot {
	return Snapshot{
		TotalConns:  t.TotalConns.Load(),
		ActiveConns: t.ActiveConns.Load(),
		BytesIn:     t.BytesIn.Load(),
		BytesOut:    t.BytesOut.Load(),
	}
}

// AddConn 增加连接
func (t *Tracker) AddConn() {
	t.TotalConns.Add(1)
	t.ActiveConns.Add(1)
}

// DoneConn 减少连接
func (t *Tracker) DoneConn() {
	t.ActiveConns.Add(-1)
}

// AddBytes 增加流量
func (t *Tracker) AddBytes(in, out int64) {
	if in > 0 {
		t.BytesIn.Add(in)
	}
	if out > 0 {
		t.BytesOut.Add(out)
	}
}
