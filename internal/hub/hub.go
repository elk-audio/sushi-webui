// Package hub serves the browser WebSocket (docs/PROTOCOL.md) and fans engine output out to clients.
package hub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/elk-audio/sushi-webui/internal/core"
)

type frame struct {
	binary bool
	data   []byte
}

type client struct {
	id   string
	out  chan frame
	drop atomic.Bool // set when the queue overflowed; the client gets a fresh state
}

type Hub struct {
	eng      *core.Engine
	insecure bool

	mu      sync.Mutex
	clients map[*client]struct{}
	seq     atomic.Int64
}

// New creates a hub. insecureOrigins disables the WebSocket Origin check (dev server proxying).
func New(eng *core.Engine, insecureOrigins bool) *Hub {
	return &Hub{eng: eng, insecure: insecureOrigins, clients: map[*client]struct{}{}}
}

func (h *Hub) BroadcastJSON(v any) {
	var b []byte
	switch x := v.(type) {
	case json.RawMessage:
		b = x
	default:
		var err error
		if b, err = json.Marshal(v); err != nil {
			slog.Error("marshal broadcast", "err", err)
			return
		}
	}
	h.broadcast(frame{false, b})
}

func (h *Hub) BroadcastBinary(b []byte) { h.broadcast(frame{true, b}) }

func (h *Hub) broadcast(f frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c.out <- f:
		default:
			// Meter frames can simply be dropped; for JSON, resync the client with a full state.
			if !f.binary {
				c.drop.Store(true)
			}
		}
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: h.insecure})
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)
	c := &client{id: fmt.Sprintf("c%d", h.seq.Add(1)), out: make(chan frame, 512)}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	hello, _ := json.Marshal(map[string]any{"t": "hello", "proto": 1, "client": c.id})
	c.out <- frame{false, hello}
	c.out <- frame{false, h.eng.StateMessage()}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}()
	slog.Info("client connected", "client", c.id, "remote", r.RemoteAddr)

	go h.writer(ctx, cancel, conn, c)
	h.reader(ctx, conn, c)
	conn.Close(websocket.StatusNormalClosure, "")
	slog.Info("client disconnected", "client", c.id)
}

func (h *Hub) writer(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, c *client) {
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case f := <-c.out:
			if c.drop.Swap(false) {
				// drain queued JSON and send a fresh state instead
				for len(c.out) > 0 {
					<-c.out
				}
				f = frame{false, h.eng.StateMessage()}
			}
			typ := websocket.MessageText
			if f.binary {
				typ = websocket.MessageBinary
			}
			wctx, wcancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, typ, f.data)
			wcancel()
			if err != nil {
				return
			}
		}
	}
}

type inMsg struct {
	T     string          `json:"t"`
	ID    int64           `json:"id"`
	Op    string          `json:"op"`
	Args  json.RawMessage `json:"args"`
	P     int32           `json:"p"`
	K     int32           `json:"k"`
	N     float32         `json:"n"`
	Track int32           `json:"track"`
	Bus   int32           `json:"bus"`
	DB    float32         `json:"db"`
}

func (h *Hub) reader(ctx context.Context, conn *websocket.Conn, c *client) {
	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var m inMsg
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		switch m.T {
		case "set":
			h.eng.Set(m.P, m.K, m.N)
		case "send":
			h.eng.SetSendLevel(m.Track, m.Bus, m.DB)
		case "cmd":
			go func(m inMsg) {
				data, err := h.eng.Command(m.Op, m.Args)
				res := map[string]any{"t": "result", "id": m.ID, "ok": err == nil}
				if data != nil {
					res["data"] = data
				}
				if err != nil {
					var ue *core.UserError
					if errors.As(err, &ue) {
						res["error"] = ue.Msg
					} else {
						res["error"] = err.Error()
					}
				}
				b, _ := json.Marshal(res)
				select {
				case c.out <- frame{false, b}:
				case <-ctx.Done():
				}
			}(m)
		}
	}
}
