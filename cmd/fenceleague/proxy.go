package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Proxy sits between every match's bridge and the model endpoints. It is where the league
// counts calls, tokens and latency per match, caps each endpoint's concurrency, and holds
// a yielding endpoint's requests while the owner plays.
//
// A bridge's base URL is http://<proxy>/m/<match>/v1; the match picks the endpoint.
type Proxy struct {
	gate   *GameGate
	client *http.Client

	mu     sync.Mutex
	routes map[string]*route // match id → route
	limits map[string]*Slots
	addr   string
	server *http.Server
}

type route struct {
	endpoint string
	ep       *Endpoint
	log      *os.File
	enc      *json.Encoder
	mu       sync.Mutex
}

// CallRecord is one line of a match's llm.jsonl.
type CallRecord struct {
	TS               time.Time `json:"ts"`
	Endpoint         string    `json:"endpoint"`
	Path             string    `json:"path"`
	Status           int       `json:"status"`
	QueueMS          int64     `json:"queue_ms"`
	LatencyMS        int64     `json:"latency_ms"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	Error            string    `json:"error,omitempty"`
	Retried          bool      `json:"retried,omitempty"`
}

// retryAfter is the pause before the one retry of an HTTP 507.
var retryAfter = 5 * time.Second

func NewProxy(gate *GameGate, endpoints map[string]*Endpoint, concurrency map[string]int) (*Proxy, error) {
	p := &Proxy{
		gate:   gate,
		client: &http.Client{Timeout: 10 * time.Minute},
		routes: map[string]*route{},
		limits: map[string]*Slots{},
	}
	for name := range endpoints {
		s, err := NewSlots(slotRoot, name, concurrency[name])
		if err != nil {
			return nil, err
		}
		p.limits[name] = s
	}
	return p, nil
}

// slotRoot holds the endpoint slots every league on this machine shares.
var slotRoot = filepath.Join(leagueRoot, "slots")

func (p *Proxy) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	p.addr = ln.Addr().String()
	p.server = &http.Server{Handler: p, ReadHeaderTimeout: 10 * time.Second}
	go p.server.Serve(ln)
	return nil
}

func (p *Proxy) Close() {
	if p.server != nil {
		p.server.Close()
	}
}

// BaseURL is what match's bridge uses as -base-url.
func (p *Proxy) BaseURL(match string) string { return "http://" + p.addr + "/m/" + match + "/v1" }

// Register routes match to endpoint and appends its calls to logPath.
func (p *Proxy) Register(match, endpoint string, ep *Endpoint, logPath string) error {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.routes[match] = &route{endpoint: endpoint, ep: ep, log: f, enc: json.NewEncoder(f)}
	return nil
}

func (p *Proxy) Unregister(match string) {
	p.mu.Lock()
	r := p.routes[match]
	delete(p.routes, match)
	p.mu.Unlock()
	if r != nil {
		r.mu.Lock()
		r.log.Close()
		r.mu.Unlock()
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rest, ok := strings.CutPrefix(req.URL.Path, "/m/")
	match, suffix, _ := strings.Cut(rest, "/v1")
	p.mu.Lock()
	r := p.routes[match]
	limit := p.limits[routeEndpoint(r)]
	p.mu.Unlock()
	if !ok || r == nil || limit == nil {
		http.Error(w, "unknown match", http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rec := CallRecord{TS: time.Now().UTC(), Endpoint: r.endpoint, Path: suffix}
	queued := time.Now()
	if r.ep.YieldsToGame {
		if err := p.gate.WaitClear(req.Context()); err != nil {
			return
		}
	}
	release, err := limit.Acquire(req.Context())
	if err != nil {
		return
	}
	defer release()
	rec.QueueMS = time.Since(queued).Milliseconds()

	start := time.Now()
	status, respBody, header, err := p.forward(req.Context(), r.ep.BaseURL+suffix, req, body)
	if err == nil && status == http.StatusInsufficientStorage {
		// drift's MLX server refuses a prompt its memory cannot fit right now; its session
		// cache usually frees enough a moment later.
		rec.Retried = true
		select {
		case <-time.After(retryAfter):
		case <-req.Context().Done():
			return
		}
		status, respBody, header, err = p.forward(req.Context(), r.ep.BaseURL+suffix, req, body)
	}
	rec.LatencyMS = time.Since(start).Milliseconds()
	rec.Status = status
	if err != nil {
		rec.Error = err.Error()
		r.write(rec)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	var u struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(respBody, &u) == nil {
		rec.PromptTokens, rec.CompletionTokens = u.Usage.PromptTokens, u.Usage.CompletionTokens
	}
	if status != http.StatusOK {
		rec.Error = clip(string(respBody), 300)
	}
	r.write(rec)
	if ct := header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.WriteHeader(status)
	w.Write(respBody)
}

func routeEndpoint(r *route) string {
	if r == nil {
		return ""
	}
	return r.endpoint
}

func (p *Proxy) forward(ctx context.Context, url string, in *http.Request, body []byte) (int, []byte, http.Header, error) {
	out, err := http.NewRequestWithContext(ctx, in.Method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	for _, h := range []string{"Content-Type", "Authorization", "Accept"} {
		if v := in.Header.Get(h); v != "" {
			out.Header.Set(h, v)
		}
	}
	resp, err := p.client.Do(out)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp.StatusCode, b, resp.Header, err
}

func (r *route) write(rec CallRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.enc.Encode(rec)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
