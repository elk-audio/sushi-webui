package core_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elk-audio/sushi-webui/internal/catalog"
	"github.com/elk-audio/sushi-webui/internal/core"
	"github.com/elk-audio/sushi-webui/internal/sushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/fakesushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
)

// sink records what the engine broadcasts, standing in for the WebSocket hub.
type sink struct {
	mu     sync.Mutex
	json   []string
	binary int
}

func (s *sink) BroadcastJSON(v any) {
	b, _ := json.Marshal(v)
	s.mu.Lock()
	s.json = append(s.json, string(b))
	s.mu.Unlock()
}
func (s *sink) BroadcastBinary([]byte) { s.mu.Lock(); s.binary++; s.mu.Unlock() }
func (s *sink) sawJSON(sub string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.json {
		if strings.Contains(m, sub) {
			return true
		}
	}
	return false
}
func (s *sink) binaries() int { s.mu.Lock(); defer s.mu.Unlock(); return s.binary }

type view struct {
	State struct {
		Sushi struct {
			Connected bool   `json:"connected"`
			Version   string `json:"version"`
		} `json:"sushi"`
		Tracks []struct {
			ID    int32  `json:"id"`
			Name  string `json:"name"`
			Kind  string `json:"kind"`
			Meter int    `json:"meter"`
		} `json:"tracks"`
		Master *struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"master"`
	} `json:"state"`
}

func state(e *core.Engine) view {
	var v view
	_ = json.Unmarshal(e.StateMessage(), &v)
	return v
}

func (v view) hasMaster() bool { return v.State.Master != nil && v.State.Master.Kind == "post" }

func (v view) track(name string) (id int32, kind string, ok bool) {
	for _, t := range v.State.Tracks {
		if t.Name == name {
			return t.ID, t.Kind, true
		}
	}
	return 0, "", false
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type rig struct {
	f   *fakesushi.Fake
	e   *core.Engine
	s   *sink
	zmq *fakesushi.ZMQServer // nil for gRPC
}

func startEngine(t *testing.T, transport string) *rig {
	t.Helper()
	f := fakesushi.New()
	f.AddTrack("Audio 1", rpc.TrackType_REGULAR)
	r := &rig{f: f, s: &sink{}}
	var d sushi.Dialer
	if transport == "zmq" {
		r.zmq = fakesushi.StartZMQ(t, f)
		d = r.zmq.ZMQDialer(zmqconn.Config{KeepAlive: 200 * time.Millisecond, CallTimeout: time.Second})
	} else {
		d = fakesushi.ServeGRPC(t, f)
	}
	dir, err := os.MkdirTemp("", "e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	r.e = core.New(d, dir, catalog.Build("", nil, nil, nil))
	r.e.SetSink(r.s)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.e.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return r
}

func forBoth(t *testing.T, body func(t *testing.T, r *rig)) {
	for _, tr := range []string{"grpc", "zmq"} {
		t.Run(tr, func(t *testing.T) { body(t, startEngine(t, tr)) })
	}
}

func TestEngineConnectsAndBuildsModel(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		eventually(t, "tracks", func() bool {
			v := state(r.e)
			_, _, a := v.track("Audio 1")
			return v.State.Sushi.Connected && a && v.hasMaster()
		})
		if v := state(r.e); v.State.Sushi.Version != "1.4.0-fake" {
			t.Fatalf("version = %q", v.State.Sushi.Version)
		}
		// The engine's invariant: every track ends in a hidden meter, and Master was created.
		eventually(t, "meters on both tracks", func() bool {
			return len(r.f.ProcessorNames("Audio 1")) == 1 && len(r.f.ProcessorNames("Master")) == 1
		})
		if n := r.f.ProcessorNames("Audio 1")[0]; !strings.HasPrefix(n, "__meter__") {
			t.Fatalf("hidden processor name = %q", n)
		}
	})
}

func TestEngineAddsTracks(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		eventually(t, "connected", func() bool { return state(r.e).hasMaster() })
		res, err := r.e.Command("addTrack", json.RawMessage(`{"kind":"audio","name":"Guitar","channels":2}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = res
		eventually(t, "Guitar in the model", func() bool { _, k, ok := state(r.e).track("Guitar"); return ok && k == "audio" })
		eventually(t, "Guitar meter", func() bool { return len(r.f.ProcessorNames("Guitar")) == 1 })
	})
}

func TestEngineForwardsNotifications(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		eventually(t, "connected", func() bool { return state(r.e).hasMaster() })
		id, _, _ := state(r.e).track("Audio 1")

		// Fader moved by someone else (another client, MIDI): it must reach the browser as a params update.
		r.f.SetParam(id, fakesushi.TrackGain, 0.5)
		eventually(t, "params update", func() bool { return r.s.sawJSON(`"t":"params"`) })

		if _, err := r.f.TransportTempo(140); err != nil {
			t.Fatal(err)
		}
		eventually(t, "transport update", func() bool { return r.s.sawJSON(`"t":"transport"`) && r.s.sawJSON(`140`) })

		// Meters come in as parameter updates of the hidden peakmeter and leave as binary frames.
		eventually(t, "meter processor", func() bool { return len(r.f.ProcessorNames("Audio 1")) == 1 })
		mp := r.f.ProcessorID(r.f.ProcessorNames("Audio 1")[0])
		before := r.s.binaries()
		for i := 0; i < 20 && r.s.binaries() == before; i++ {
			r.f.SetParam(mp, fakesushi.MeterParam("level_0"), -12+float32(i))
			time.Sleep(40 * time.Millisecond)
		}
		if r.s.binaries() == before {
			t.Fatal("no meter frame was broadcast")
		}
	})
}

// Over ZeroMQ nothing on the sockets announces that Sushi died; the keepalive has to end the
// session so the supervisor reconnects once Sushi is back.
func TestEngineReconnectsOverZMQ(t *testing.T) {
	r := startEngine(t, "zmq")
	eventually(t, "connected", func() bool { return state(r.e).State.Sushi.Connected })
	control, notify := r.zmq.Control, r.zmq.Notify
	r.zmq.Close()
	eventually(t, "disconnect noticed", func() bool { return r.s.sawJSON(`"sushi":false`) })
	fakesushi.StartZMQAt(t, r.f, control, notify)
	eventually(t, "reconnected", func() bool {
		return state(r.e).State.Sushi.Connected && r.f.Calls("GetSushiVersion") >= 2
	})
}
