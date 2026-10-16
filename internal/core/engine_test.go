package core_test

import (
	"context"
	"encoding/json"
	"fmt"
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
			ID   int32  `json:"id"`
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
	return startEngineWith(t, transport, "Audio 1")
}

func startEngineWith(t *testing.T, transport string, tracks ...string) *rig {
	t.Helper()
	f := fakesushi.New()
	for _, n := range tracks {
		f.AddTrack(n, rpc.TrackType_REGULAR)
	}
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

// pluginsByRole picks n distinct catalog entries with that role.
func pluginsByRole(t *testing.T, role catalog.Role, n int) []string {
	t.Helper()
	var ids []string
	for _, e := range catalog.Build("", nil, nil, nil).Visible() {
		if e.Role == role && len(ids) < n {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) < n {
		t.Fatalf("catalog has only %d %s plugins", len(ids), role)
	}
	return ids
}

func (r *rig) cmd(t *testing.T, op string, args map[string]any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(args)
	res, err := r.e.Command(op, b)
	if err != nil {
		t.Fatalf("%s: %v", op, err)
	}
	return res
}

func (r *rig) cmdErr(t *testing.T, op string, args map[string]any) error {
	t.Helper()
	b, _ := json.Marshal(args)
	_, err := r.e.Command(op, b)
	if err == nil {
		t.Fatalf("%s should have failed", op)
	}
	return err
}

// chain is a track's processors in Sushi's order, hidden ones included (the meter is last).
func (r *rig) chain(track string) string { return strings.Join(r.f.ProcessorNames(track), ",") }

func (r *rig) addPlugin(t *testing.T, track int32, catalogID string) int32 {
	t.Helper()
	return int32(r.cmd(t, "addPlugin", map[string]any{"track": track, "catalogId": catalogID})["id"].(int32))
}

func TestEngineRemovesAndReordersPlugins(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		eventually(t, "connected", func() bool { _, _, ok := state(r.e).track("Audio 1"); return ok && state(r.e).hasMaster() })
		tid, _, _ := state(r.e).track("Audio 1")
		fx := pluginsByRole(t, catalog.AudioFx, 3)
		a, b, c := r.addPlugin(t, tid, fx[0]), r.addPlugin(t, tid, fx[1]), r.addPlugin(t, tid, fx[2])
		names := r.f.ProcessorNames("Audio 1")
		if len(names) != 4 || !strings.HasPrefix(names[3], "__meter__") {
			t.Fatalf("chain = %v, want 3 plugins before the meter", names)
		}
		nameA, nameB, nameC := names[0], names[1], names[2]

		// A hidden processor isn't a plugin: the browser can't delete or move it.
		meter := r.f.ProcessorID(names[3])
		r.cmdErr(t, "removePlugin", map[string]any{"proc": meter})
		r.cmdErr(t, "movePlugin", map[string]any{"proc": meter, "index": 0})
		r.cmdErr(t, "removePlugin", map[string]any{"proc": 9999})

		r.cmd(t, "movePlugin", map[string]any{"proc": c, "index": 0})
		if got, want := r.chain("Audio 1"), strings.Join([]string{nameC, nameA, nameB, names[3]}, ","); got != want {
			t.Fatalf("after moving to the front: %s, want %s", got, want)
		}
		// Past the end means last visible slot: still before the meter.
		r.cmd(t, "movePlugin", map[string]any{"proc": c, "index": 99})
		if got, want := r.chain("Audio 1"), strings.Join([]string{nameA, nameB, nameC, names[3]}, ","); got != want {
			t.Fatalf("after moving to the end: %s, want %s", got, want)
		}

		r.cmd(t, "removePlugin", map[string]any{"proc": b})
		if got, want := r.chain("Audio 1"), strings.Join([]string{nameA, nameC, names[3]}, ","); got != want {
			t.Fatalf("after removing: %s, want %s", got, want)
		}
		eventually(t, "removed plugin gone from the model", func() bool {
			return !strings.Contains(string(r.e.StateMessage()), `"id":`+fmt.Sprint(b)+`,"name":"`+nameB)
		})
		_ = a
	})
}

func TestEngineKeepsMidiSectionsOrdered(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		eventually(t, "connected", func() bool { return state(r.e).hasMaster() })
		r.cmd(t, "addTrack", map[string]any{"kind": "midi", "name": "Keys", "channels": 2})
		var tid int32
		eventually(t, "Keys", func() bool { id, _, ok := state(r.e).track("Keys"); tid = id; return ok })
		mfx := r.addPlugin(t, tid, pluginsByRole(t, catalog.MidiFx, 1)[0])
		inst := r.addPlugin(t, tid, pluginsByRole(t, catalog.Instrument, 1)[0])
		fx := r.addPlugin(t, tid, pluginsByRole(t, catalog.AudioFx, 1)[0])
		before := r.chain("Keys")

		// The generator can't go after the effect, nor ahead of the MIDI effect; the effect can't jump ahead of it.
		r.cmdErr(t, "movePlugin", map[string]any{"proc": inst, "index": 2})
		r.cmdErr(t, "movePlugin", map[string]any{"proc": inst, "index": 0})
		r.cmdErr(t, "movePlugin", map[string]any{"proc": fx, "index": 0})
		r.cmdErr(t, "movePlugin", map[string]any{"proc": mfx, "index": 2})
		if got := r.chain("Keys"); got != before {
			t.Fatalf("a rejected move changed the chain: %s, want %s", got, before)
		}

		// With the instrument gone, a new one can be added again (it goes between the MIDI and audio effects).
		r.cmd(t, "removePlugin", map[string]any{"proc": inst})
		r.addPlugin(t, tid, pluginsByRole(t, catalog.Instrument, 1)[0])
		names := r.f.ProcessorNames("Keys")
		if len(names) != 4 || names[0] != before[:strings.Index(before, ",")] {
			t.Fatalf("chain = %v", names)
		}
	})
}

func TestEngineRemovesTracks(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		eventually(t, "connected", func() bool { return state(r.e).hasMaster() })
		r.cmd(t, "addTrack", map[string]any{"kind": "audio", "name": "Guitar", "channels": 2})
		var tid int32
		eventually(t, "Guitar", func() bool { id, _, ok := state(r.e).track("Guitar"); tid = id; return ok })
		r.addPlugin(t, tid, pluginsByRole(t, catalog.AudioFx, 1)[0])

		r.cmd(t, "removeTrack", map[string]any{"track": tid})
		eventually(t, "Guitar gone", func() bool { _, _, ok := state(r.e).track("Guitar"); return !ok })
		if got := r.f.ProcessorNames("Guitar"); len(got) != 0 {
			t.Fatalf("processors left behind: %v", got)
		}

		// Master can't be deleted.
		r.cmdErr(t, "removeTrack", map[string]any{"track": state(r.e).State.Master.ID})
	})
}

// Sushi's JSON schema rejects an empty track list, so configs carry a placeholder track. The backend
// deletes it before the first state goes out, and the project starts empty.
func TestEngineHidesPlaceholderTrack(t *testing.T) {
	for _, tr := range []string{"grpc", "zmq"} {
		t.Run(tr, func(t *testing.T) {
			r := startEngineWith(t, tr, "__placeholder__")
			eventually(t, "connected", func() bool { return state(r.e).hasMaster() })
			eventually(t, "placeholder deleted", func() bool { return r.f.ProcessorID("__placeholder__") < 0 && len(r.f.TrackNames()) == 1 })
			if n := len(state(r.e).State.Tracks); n != 0 {
				t.Fatalf("%d tracks in the model, want 0", n)
			}
			if r.s.sawJSON("__placeholder__") {
				t.Fatal("the placeholder reached the browser")
			}
			// An empty project accepts its first track.
			r.cmd(t, "addTrack", map[string]any{"kind": "audio", "name": "Guitar", "channels": 2})
			eventually(t, "Guitar", func() bool { _, _, ok := state(r.e).track("Guitar"); return ok })
		})
	}
}

func TestEngineRoutesTracks(t *testing.T) {
	forBoth(t, func(t *testing.T, r *rig) {
		r.e.SetDefaultBuses(1, 2) // new tracks play to outputs 3-4
		eventually(t, "connected", func() bool { return state(r.e).hasMaster() })
		r.cmd(t, "addTrack", map[string]any{"kind": "audio", "name": "Guitar", "channels": 2})
		r.cmd(t, "addTrack", map[string]any{"kind": "audio", "name": "Vox", "channels": 1, "input": 1})
		var gid, vid int32
		route := func(name string) (in, out int, mono bool, inTxt, outTxt string) {
			var v struct {
				State struct {
					Tracks []struct {
						Name   string `json:"name"`
						Input  string `json:"input"`
						Output string `json:"output"`
						InChan int    `json:"inChan"`
						OutCh  int    `json:"outChan"`
						InMono bool   `json:"inMono"`
					} `json:"tracks"`
				} `json:"state"`
			}
			_ = json.Unmarshal(r.e.StateMessage(), &v)
			for _, tr := range v.State.Tracks {
				if tr.Name == name {
					return tr.InChan, tr.OutCh, tr.InMono, tr.Input, tr.Output
				}
			}
			return -9, -9, false, "", ""
		}
		eventually(t, "tracks", func() bool {
			var a, b bool
			gid, _, a = state(r.e).track("Guitar")
			vid, _, b = state(r.e).track("Vox")
			_, out, _, _, _ := route("Guitar")
			return a && b && out == 2
		})
		if in, out, mono, inTxt, outTxt := route("Guitar"); in != 0 || out != 2 || mono || inTxt != "In 1-2" || outTxt != "Out 3-4" {
			t.Fatalf("Guitar routing = %d %d %v %q %q", in, out, mono, inTxt, outTxt)
		}
		if in, _, mono, inTxt, _ := route("Vox"); in != 1 || !mono || inTxt != "In 2" {
			t.Fatalf("Vox routing = %d %v %q", in, mono, inTxt)
		}

		r.cmd(t, "setRouting", map[string]any{"track": gid, "output": 0})
		eventually(t, "Guitar to outputs 1-2", func() bool { _, out, _, _, txt := route("Guitar"); return out == 0 && txt == "Out 1-2" })
		r.cmd(t, "setRouting", map[string]any{"track": vid, "input": 0})
		eventually(t, "Vox to input 1", func() bool { in, _, mono, txt, _ := route("Vox"); return in == 0 && mono && txt == "In 1" })

		r.cmdErr(t, "setRouting", map[string]any{"track": gid, "output": 6}) // beyond the device
		r.cmdErr(t, "setRouting", map[string]any{"track": gid, "output": 1}) // not a pair boundary
		r.cmdErr(t, "setRouting", map[string]any{"track": gid, "input": 1})  // stereo needs a pair
		r.cmdErr(t, "setRouting", map[string]any{"track": state(r.e).State.Master.ID, "output": 0})
	})
}
