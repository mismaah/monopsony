// Package metrics exposes the server's Prometheus instruments. Everything
// is registered on a private registry so tests can create isolated sets and
// so the /metrics endpoint only ever shows our own series (plus Go runtime).
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics is the set of instruments the packages record into.
type Metrics struct {
	reg *prometheus.Registry

	HTTPRequests *prometheus.CounterVec
	HTTPDuration *prometheus.HistogramVec
	RateLimited  *prometheus.CounterVec

	WSConnections prometheus.Gauge
	WSMessages    *prometheus.CounterVec

	Commands   *prometheus.CounterVec
	Events     *prometheus.CounterVec
	BotActions prometheus.Counter
	Timeouts   prometheus.Counter

	ClusterRPC *prometheus.CounterVec
}

// New builds a registry with the standard Go/process collectors.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	f := factory{reg}
	return &Metrics{
		reg:          reg,
		HTTPRequests: f.counterVec("monopsony_http_requests_total", "HTTP requests by route, method and status.", []string{"route", "method", "status"}),
		HTTPDuration: f.histogramVec("monopsony_http_request_duration_seconds", "HTTP request latency by route.", []string{"route"},
			[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5}),
		RateLimited:   f.counterVec("monopsony_rate_limited_total", "Requests or frames rejected by a rate limiter.", []string{"scope"}),
		WSConnections: f.gauge("monopsony_ws_connections", "Open WebSocket connections."),
		WSMessages:    f.counterVec("monopsony_ws_messages_total", "Inbound WebSocket frames by type.", []string{"type"}),
		Commands:      f.counterVec("monopsony_game_commands_total", "Game commands applied, by type and result.", []string{"type", "result"}),
		Events:        f.counterVec("monopsony_game_events_total", "Game events emitted by the engine.", []string{"type"}),
		BotActions:    f.counter("monopsony_bot_actions_total", "Commands issued by bots."),
		Timeouts:      f.counter("monopsony_turn_timeouts_total", "Human decisions replaced by the safe default after a timeout."),
		ClusterRPC:    f.counterVec("monopsony_cluster_rpc_total", "Cross-node room RPCs by method and result.", []string{"method", "result"}),
	}
}

// RegisterRooms adds gauges that report this node's live rooms per status
// at scrape time.
func (m *Metrics) RegisterRooms(count func() map[string]int) {
	for _, s := range []struct{ status, help string }{
		{"lobby", "Rooms waiting in the lobby on this node."},
		{"in_progress", "Games in progress on this node."},
		{"finished", "Finished games still held in memory on this node."},
	} {
		status := s.status
		m.reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "monopsony_rooms_" + status, Help: s.help},
			func() float64 { return float64(count()[status]) }))
	}
}

// Handler serves the registry in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{})
}

// Registry exposes the registry for extra collectors.
func (m *Metrics) Registry() *prometheus.Registry { return m.reg }

// HTTP wraps a ServeMux, recording a request counter and latency per
// matched pattern. The label uses the mux pattern (bounded cardinality),
// never the raw path.
func (m *Metrics) HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		m.HTTPRequests.WithLabelValues(route, r.Method, strconv.Itoa(sw.status)).Inc()
		m.HTTPDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
	})
}

// statusWriter captures the status code. Unwrap lets http.ResponseController
// reach Hijack/Flush on the underlying writer so the WebSocket upgrade keeps
// working through the middleware.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusWriter) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	s.wrote = true
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// factory is a tiny helper so New reads as a table.
type factory struct{ reg *prometheus.Registry }

func (p factory) counter(name, help string) prometheus.Counter {
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: help})
	p.reg.MustRegister(c)
	return c
}

func (p factory) counterVec(name, help string, labels []string) *prometheus.CounterVec {
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	p.reg.MustRegister(c)
	return c
}

func (p factory) gauge(name, help string) prometheus.Gauge {
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name, Help: help})
	p.reg.MustRegister(g)
	return g
}

func (p factory) histogramVec(name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: name, Help: help, Buckets: buckets}, labels)
	p.reg.MustRegister(h)
	return h
}

var nop = New()

// Default is a shared, never-exported instance so packages can record
// without nil checks when no registry was injected.
func Default() *Metrics { return nop }
