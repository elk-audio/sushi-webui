// Package core mirrors Sushi's state, adds the WebUI concepts Sushi lacks (track kinds, buses,
// sends, solo, meters) and turns browser commands into gRPC calls. See ARCHITECTURE.md §4.
package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/elk-audio/sushi-webui/internal/catalog"
	"github.com/elk-audio/sushi-webui/internal/sushi"
	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
)

// Sink receives everything the engine broadcasts. Implemented by the WebSocket hub.
type Sink interface {
	BroadcastJSON(v any)
	BroadcastBinary(b []byte)
}

type meterRef struct {
	track int32
	ch    int
	clip  bool
}

type meterSlot struct {
	pre   [2]float32 // pre-fader dB from the peakmeter
	clip  [2]bool
	dirty bool
}

type sendKey struct{ track, bus int32 }

type Engine struct {
	dialer   sushi.Dialer
	dataDir  string
	cat      *catalog.Catalog
	audioDir string // base folder for Wav Streamer files
	sink     Sink

	opMu sync.Mutex // serializes graph operations and refreshes

	mu         sync.Mutex // guards everything below
	c          *sushi.Client
	st         State
	rev        uint64
	meta       *Meta
	params     map[pkey]*Param
	meterRefs  map[pkey]meterRef
	tracks     map[int32]*Track // all tracks incl. pre/post, by id
	trackOf    map[int32]int32  // processor id (visible or hidden) -> track id
	slots      map[int32]int    // track id -> meter slot (stable while running)
	meters     map[int]*meterSlot
	procCache  map[int32]*Proc            // visible processors by id, keeps param objects across refreshes
	hidParams  map[int32]map[string]int32 // hidden processor id -> param name -> id
	muteKeys   map[pkey]int32             // track mute param -> track id
	mixKeys    map[pkey]int32             // track gain/pan param -> track id
	batch      [][]any
	pendSet    map[pkey]float32
	pendSend   map[sendKey]float32
	sendsDirty bool
	metaDirty  bool
	stateDirty bool

	refreshReq chan struct{}
}

func New(dialer sushi.Dialer, dataDir string, cat *catalog.Catalog) *Engine {
	e := &Engine{
		dialer: dialer, dataDir: dataDir, cat: cat,
		params: map[pkey]*Param{}, meterRefs: map[pkey]meterRef{}, tracks: map[int32]*Track{}, trackOf: map[int32]int32{},
		slots: map[int32]int{}, meters: map[int]*meterSlot{}, procCache: map[int32]*Proc{}, hidParams: map[int32]map[string]int32{},
		muteKeys: map[pkey]int32{}, mixKeys: map[pkey]int32{}, pendSet: map[pkey]float32{}, pendSend: map[sendKey]float32{},
		refreshReq: make(chan struct{}, 1),
	}
	e.meta = loadMeta(e.metaPath())
	e.st.Catalog = cat.Visible()
	e.st.Project = ProjectInfo{Name: e.meta.Project, List: listProjects(dataDir)}
	e.st.UI = e.meta.UI
	e.st.Tracks = []*Track{}
	return e
}

func (e *Engine) SetSink(s Sink) { e.sink = s }

func (e *Engine) metaPath() string { return filepath.Join(e.dataDir, "current.meta.json") }

// StateMessage returns the full state message for a newly connected client.
func (e *Engine) StateMessage() []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"t": "state", "rev": e.rev, "state": &e.st})
	return b
}

func (e *Engine) client() *sushi.Client {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.c
}

func (e *Engine) markState() { e.mu.Lock(); e.stateDirty = true; e.mu.Unlock() }

func (e *Engine) requestRefresh() {
	select {
	case e.refreshReq <- struct{}{}:
	default:
	}
}

// ---------------------------------------------------------------- run loop

// Run keeps a session with Sushi alive, reconnecting with backoff, until ctx ends.
func (e *Engine) Run(ctx context.Context) {
	go e.tickers(ctx)
	backoff := 500 * time.Millisecond
	for ctx.Err() == nil {
		start := time.Now()
		err := e.session(ctx)
		e.mu.Lock()
		e.c = nil
		e.st.Sushi.Connected = false
		e.stateDirty = true
		e.mu.Unlock()
		if e.sink != nil {
			e.sink.BroadcastJSON(map[string]any{"t": "conn", "sushi": false})
		}
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > 10*time.Second {
			backoff = 500 * time.Millisecond
		}
		slog.Warn("sushi connection lost, retrying", "sushi", e.dialer.Name, "err", err, "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

func (e *Engine) session(ctx context.Context) error {
	c, err := e.dialer.Dial()
	if err != nil {
		return err
	}
	defer c.Close()
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cctx, ccancel := sushi.Ctx()
	ver, err := c.System.GetSushiVersion(cctx, sushi.Void)
	ccancel()
	if err != nil {
		return err
	}
	info := SushiInfo{Connected: true, Version: ver.Value}
	if api, err := c.System.GetSushiApiVersion(ctxT(), sushi.Void); err == nil {
		if !strings.HasPrefix(api.Value, "1.") {
			slog.Warn("untested Sushi API version", "api", api.Value)
		}
	}
	if v, err := c.System.GetInputAudioChannelCount(ctxT(), sushi.Void); err == nil {
		info.Inputs = int(v.Value)
	}
	if v, err := c.System.GetOutputAudioChannelCount(ctxT(), sushi.Void); err == nil {
		info.Outputs = int(v.Value)
	}
	if v, err := c.Midi.GetInputPorts(ctxT(), sushi.Void); err == nil {
		info.MidiInputs = int(v.Value)
	}
	if v, err := c.Transport.GetSamplerate(ctxT(), sushi.Void); err == nil {
		info.Samplerate = v.Value
	}
	slog.Info("connected to sushi", "version", ver.Value, "inputs", info.Inputs, "outputs", info.Outputs)
	_, _ = c.Timing.SetTimingsEnabled(ctxT(), &rpc.GenericBoolValue{Value: true})

	errCh := make(chan error, 8)
	go func() { errCh <- c.RunAsyncListener(sctx) }()
	go func() { errCh <- e.streamTracks(sctx, c) }()
	go func() { errCh <- e.streamProcessors(sctx, c) }()
	go func() { errCh <- e.streamParams(sctx, c) }()
	go func() { errCh <- e.streamTransport(sctx, c) }()
	go func() { errCh <- e.streamCpu(sctx, c) }()

	e.mu.Lock()
	e.c = c
	e.st.Sushi = info
	e.mu.Unlock()
	e.readTransport(c)

	e.opMu.Lock()
	e.resetStaleMeta(c)
	if err := e.setupLocked(); err != nil {
		slog.Error("initial setup failed", "err", err)
	}
	e.refreshLocked()
	e.applyMutesLocked()
	e.opMu.Unlock()

	for {
		select {
		case <-sctx.Done():
			return sctx.Err()
		case err := <-errCh:
			return err
		case <-e.refreshReq:
			time.Sleep(40 * time.Millisecond) // debounce bursts of add/delete notifications
			select {
			case <-e.refreshReq:
			default:
			}
			e.opMu.Lock()
			e.refreshLocked()
			e.opMu.Unlock()
		}
	}
}

// ctxT returns a context for one unary call. The deadline releases its resources, so callers don't
// need to cancel it.
func ctxT() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	time.AfterFunc(3*time.Second, cancel)
	return ctx
}

// ---------------------------------------------------------------- notification streams

func (e *Engine) streamTracks(ctx context.Context, c *sushi.Client) error {
	s, err := c.Notify.SubscribeToTrackChanges(ctx, sushi.Void)
	if err != nil {
		return err
	}
	for {
		if _, err := s.Recv(); err != nil {
			return err
		}
		e.requestRefresh()
	}
}

func (e *Engine) streamProcessors(ctx context.Context, c *sushi.Client) error {
	s, err := c.Notify.SubscribeToProcessorChanges(ctx, sushi.Void)
	if err != nil {
		return err
	}
	for {
		if _, err := s.Recv(); err != nil {
			return err
		}
		e.requestRefresh()
	}
}

func (e *Engine) streamParams(ctx context.Context, c *sushi.Client) error {
	s, err := c.Notify.SubscribeToParameterUpdates(ctx, &rpc.ParameterNotificationBlocklist{})
	if err != nil {
		return err
	}
	for {
		u, err := s.Recv()
		if err != nil {
			return err
		}
		e.onParam(u)
	}
}

func (e *Engine) onParam(u *rpc.ParameterUpdate) {
	if u.Parameter == nil {
		return
	}
	k := pkey{u.Parameter.ProcessorId, u.Parameter.ParameterId}
	e.mu.Lock()
	defer e.mu.Unlock()
	if m, ok := e.meterRefs[k]; ok {
		slot, ok := e.slots[m.track]
		if !ok {
			return
		}
		ms := e.meters[slot]
		if ms == nil {
			ms = &meterSlot{pre: [2]float32{-120, -120}}
			e.meters[slot] = ms
		}
		if m.clip {
			if u.NormalizedValue > 0.5 {
				ms.clip[m.ch] = true
			}
		} else {
			ms.pre[m.ch] = u.DomainValue
		}
		ms.dirty = true
		return
	}
	if tid, ok := e.muteKeys[k]; ok {
		if t := e.tracks[tid]; t != nil {
			t.sushiMute = u.NormalizedValue > 0.5
			if !e.soloActive() && t.Mute != t.sushiMute { // external mute changes are respected
				t.Mute = t.sushiMute
				e.meta.track(t.Name).Mute = t.Mute
				e.metaDirty, e.stateDirty = true, true
			}
			e.sendsDirty = true
		}
		return
	}
	p, ok := e.params[k]
	if !ok {
		return
	}
	p.N, p.Txt = u.NormalizedValue, u.FormattedValue
	e.batch = append(e.batch, []any{k.proc, k.param, u.NormalizedValue, u.FormattedValue})
	if _, mix := e.mixKeys[k]; mix {
		e.sendsDirty = true
		if t := e.tracks[e.mixKeys[k]]; t != nil {
			if s, ok := e.slots[t.ID]; ok && e.meters[s] != nil {
				e.meters[s].dirty = true
			}
		}
	}
}

func (e *Engine) streamTransport(ctx context.Context, c *sushi.Client) error {
	s, err := c.Notify.SubscribeToTransportChanges(ctx, sushi.Void)
	if err != nil {
		return err
	}
	for {
		u, err := s.Recv()
		if err != nil {
			return err
		}
		e.mu.Lock()
		switch v := u.Transport.(type) {
		case *rpc.TransportUpdate_Tempo:
			e.st.Transport.Tempo = v.Tempo
		case *rpc.TransportUpdate_PlayingMode:
			e.st.Transport.Playing = v.PlayingMode.GetMode() != rpc.PlayingMode_STOPPED
		}
		tr := e.st.Transport
		e.mu.Unlock()
		e.sink.BroadcastJSON(map[string]any{"t": "transport", "tempo": tr.Tempo, "playing": tr.Playing})
	}
}

func (e *Engine) readTransport(c *sushi.Client) {
	if v, err := c.Transport.GetTempo(ctxT(), sushi.Void); err == nil {
		e.mu.Lock()
		e.st.Transport.Tempo = v.Value
		e.mu.Unlock()
	}
	if v, err := c.Transport.GetPlayingMode(ctxT(), sushi.Void); err == nil {
		e.mu.Lock()
		e.st.Transport.Playing = v.GetMode() != rpc.PlayingMode_STOPPED
		e.mu.Unlock()
	}
}

func (e *Engine) streamCpu(ctx context.Context, c *sushi.Client) error {
	s, err := c.Notify.SubscribeToEngineCpuTimingUpdates(ctx, sushi.Void)
	if err != nil {
		return err
	}
	for {
		u, err := s.Recv()
		if err != nil {
			return err
		}
		v := u.GetMain().GetAverage()
		e.mu.Lock()
		e.st.Cpu = v
		e.mu.Unlock()
		e.sink.BroadcastJSON(map[string]any{"t": "cpu", "v": v})
	}
}

// ---------------------------------------------------------------- periodic work

func (e *Engine) tickers(ctx context.Context) {
	fast := time.NewTicker(10 * time.Millisecond) // param writes, send gains
	meter := time.NewTicker(33 * time.Millisecond)
	flush := time.NewTicker(20 * time.Millisecond) // param batches, state
	slow := time.NewTicker(2 * time.Second)        // metadata persistence
	defer fast.Stop()
	defer meter.Stop()
	defer flush.Stop()
	defer slow.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-fast.C:
			e.flushWrites()
		case <-meter.C:
			if b := e.meterFrame(); b != nil {
				e.sink.BroadcastBinary(b)
			}
		case <-flush.C:
			e.flushBroadcasts()
		case <-slow.C:
			e.mu.Lock()
			dirty := e.metaDirty
			e.metaDirty = false
			e.mu.Unlock()
			if dirty {
				e.mu.Lock()
				err := e.meta.save(e.metaPath())
				e.mu.Unlock()
				if err != nil {
					slog.Warn("couldn't save metadata", "err", err)
				}
			}
		}
	}
}

// FlushMeta writes pending metadata to disk; called on shutdown so the last changes aren't lost
// to the 2-second persistence tick.
func (e *Engine) FlushMeta() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.metaDirty {
		if err := e.meta.save(e.metaPath()); err != nil {
			slog.Warn("couldn't save metadata", "err", err)
		}
		e.metaDirty = false
	}
}

func (e *Engine) flushBroadcasts() {
	e.mu.Lock()
	batch := e.batch
	e.batch = nil
	var state []byte
	if e.stateDirty {
		e.stateDirty = false
		e.rev++
		state, _ = json.Marshal(map[string]any{"t": "state", "rev": e.rev, "state": &e.st})
	}
	e.mu.Unlock()
	if state != nil {
		e.sink.BroadcastJSON(json.RawMessage(state))
	}
	if len(batch) > 0 {
		e.sink.BroadcastJSON(map[string]any{"t": "params", "u": batch})
	}
}

// flushWrites sends coalesced parameter writes and recomputes post-fader send gains (§4.7).
func (e *Engine) flushWrites() {
	e.mu.Lock()
	c := e.c
	if c == nil {
		e.mu.Unlock()
		return
	}
	sets := e.pendSet
	if len(sets) > 0 {
		e.pendSet = map[pkey]float32{}
	}
	if len(e.pendSend) > 0 {
		for k, db := range e.pendSend {
			if t := e.tracks[k.track]; t != nil {
				for _, s := range t.Sends {
					if s.Bus == k.bus {
						s.DB = db
						e.meta.track(t.Name).sends(s.BusName).DB = db
					}
				}
			}
		}
		e.pendSend = map[sendKey]float32{}
		e.sendsDirty, e.metaDirty, e.stateDirty = true, true, true
	}
	if e.sendsDirty {
		e.sendsDirty = false
		solo := e.soloActive()
		for _, t := range e.tracks {
			for _, s := range t.Sends {
				want := s.DB
				if s.Post {
					if e.effectiveMute(t, solo) {
						want = -120
					} else {
						want += t.gainDb()
					}
				}
				want = clampF(want, -120, 24)
				if float32(math.Abs(float64(want-s.written))) > 0.01 || math.IsNaN(float64(s.written)) {
					s.written = want
					sets[pkey{s.proc, s.gainParam}] = (want + 120) / 144
				}
			}
		}
	}
	e.mu.Unlock()
	for k, n := range sets {
		go func(k pkey, n float32) {
			_, err := c.Param.SetParameterValue(ctxT(), &rpc.ParameterValue{Parameter: &rpc.ParameterIdentifier{ProcessorId: k.proc, ParameterId: k.param}, Value: n})
			if err != nil {
				slog.Debug("set parameter failed", "proc", k.proc, "param", k.param, "err", err)
			}
		}(k, n)
	}
}

// meterFrame builds the binary frame (PROTOCOL.md) with post-fader levels for changed slots.
func (e *Engine) meterFrame() []byte {
	e.mu.Lock()
	defer e.mu.Unlock()
	solo := e.soloActive()
	var buf []byte
	n := 0
	for tid, slot := range e.slots {
		ms := e.meters[slot]
		t := e.tracks[tid]
		if ms == nil || !ms.dirty || t == nil {
			continue
		}
		ms.dirty = false
		gl, gr := t.chGainsDb()
		l, r := ms.pre[0]+gl, ms.pre[0]+gr
		if t.Channels >= 2 || t.Kind == "post" || t.Kind == "pre" {
			r = ms.pre[1] + gr
		}
		if e.effectiveMute(t, solo) {
			l, r = -120, -120
		}
		var flags byte
		if ms.clip[0] {
			flags |= 1
		}
		if ms.clip[1] {
			flags |= 2
		}
		ms.clip = [2]bool{}
		buf = binary.LittleEndian.AppendUint16(buf, uint16(slot))
		buf = append(buf, dbToByte(l), dbToByte(r), flags)
		n++
	}
	if n == 0 {
		return nil
	}
	head := []byte{1, 0, 0}
	binary.LittleEndian.PutUint16(head[1:], uint16(n))
	return append(head, buf...)
}

func dbToByte(db float32) byte {
	v := (db + 60) / 66 * 255
	if v <= 0 || math.IsNaN(float64(v)) {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(v + 0.5)
}

func clampF(v, lo, hi float32) float32 { return max(lo, min(hi, v)) }

func (e *Engine) soloActive() bool {
	for _, t := range e.tracks {
		if t.Solo && (t.Kind == "audio" || t.Kind == "midi" || t.Kind == "bus") {
			return true
		}
	}
	return false
}

// effectiveMute is what Sushi's mute should be: user mute, or silenced by someone else's solo.
// Buses, Input and Master are solo-safe.
func (e *Engine) effectiveMute(t *Track, solo bool) bool {
	return t.Mute || (solo && (t.Kind == "audio" || t.Kind == "midi") && !t.Solo)
}

// ---------------------------------------------------------------- browser input

// Set queues a parameter write (coalesced, latest value wins).
func (e *Engine) Set(proc, param int32, n float32) {
	e.mu.Lock()
	e.pendSet[pkey{proc, param}] = clampF(n, 0, 1)
	e.st.Project.Dirty = true
	e.mu.Unlock()
}

// SetSendLevel queues a send level change in dB.
func (e *Engine) SetSendLevel(track, bus int32, db float32) {
	e.mu.Lock()
	e.pendSend[sendKey{track, bus}] = clampF(db, -120, 24)
	e.st.Project.Dirty = true
	e.mu.Unlock()
}

func (tm *TrackMeta) sends(bus string) *SendMeta {
	if tm.Sends == nil {
		tm.Sends = map[string]*SendMeta{}
	}
	s, ok := tm.Sends[bus]
	if !ok {
		s = &SendMeta{DB: -12, Post: true}
		tm.Sends[bus] = s
	}
	return s
}

// ---------------------------------------------------------------- refresh (Sushi -> model)

// refreshLocked rebuilds the graph from Sushi. Parameter info is fetched only for processors not
// seen before; their values stay current through notifications. Caller holds opMu.
func (e *Engine) refreshLocked() {
	c := e.client()
	if c == nil {
		return
	}
	all, err := c.Graph.GetAllTracks(ctxT(), sushi.Void)
	if err != nil {
		slog.Warn("refresh failed", "err", err)
		return
	}
	kbd := map[int32]*rpc.MidiKbdConnection{}
	if kc, err := c.Midi.GetAllKbdInputConnections(ctxT(), sushi.Void); err == nil {
		for _, k := range kc.Connections {
			kbd[k.GetTrack().GetId()] = k
		}
	}

	type sendInfo struct {
		track *Track
		proc  int32
		dest  string
	}
	var sends []sendInfo
	var regular, buses []*Track
	var pre, post *Track
	newParams := map[pkey]*Param{}
	newMeterRefs := map[pkey]meterRef{}
	newMute := map[pkey]int32{}
	newMix := map[pkey]int32{}
	newTrackOf := map[int32]int32{}
	newTracks := map[int32]*Track{}
	newCache := map[int32]*Proc{}
	var unknownProcs bool

	e.mu.Lock()
	oldTracks := e.tracks
	e.mu.Unlock()

	for _, ti := range all.Tracks {
		t := &Track{ID: ti.Id, Name: ti.Name, Label: ti.Name, Channels: int(ti.Channels), Chain: []*Proc{}, Sends: []*Send{}, Meter: -1}
		old := oldTracks[ti.Id]
		switch ti.GetType().GetType() {
		case rpc.TrackType_PRE:
			t.Kind = "pre"
		case rpc.TrackType_POST:
			t.Kind = "post"
		}
		e.mu.Lock()
		if tm := e.meta.Tracks[ti.Name]; tm != nil && tm.Label != "" {
			t.Label = tm.Label
		}
		e.mu.Unlock()
		// track parameters (gain, pan, mute)
		if old != nil && old.Name == t.Name {
			t.Gain, t.Pan, t.muteParam, t.sushiMute = old.Gain, old.Pan, old.muteParam, old.sushiMute
		} else {
			ps, err := c.Param.GetTrackParameters(ctxT(), &rpc.TrackIdentifier{Id: ti.Id})
			if err == nil {
				for _, pi := range ps.Parameters {
					switch pi.Name {
					case "gain":
						t.Gain = e.loadParam(c, ti.Id, pi)
					case "pan":
						t.Pan = e.loadParam(c, ti.Id, pi)
					case "mute":
						t.muteParam = pi.Id
						if v, err := c.Param.GetParameterValue(ctxT(), &rpc.ParameterIdentifier{ProcessorId: ti.Id, ParameterId: pi.Id}); err == nil {
							t.sushiMute = v.Value > 0.5
						}
					}
				}
			}
		}
		if t.Kind == "pre" || t.Kind == "post" {
			t.Pan = nil
		}
		if t.Gain != nil {
			newParams[pkey{t.ID, t.Gain.ID}] = t.Gain
			newMix[pkey{t.ID, t.Gain.ID}] = t.ID
		}
		if t.Pan != nil {
			newParams[pkey{t.ID, t.Pan.ID}] = t.Pan
			newMix[pkey{t.ID, t.Pan.ID}] = t.ID
		}
		newMute[pkey{t.ID, t.muteParam}] = t.ID

		procs, err := c.Graph.GetTrackProcessors(ctxT(), &rpc.TrackIdentifier{Id: ti.Id})
		if err != nil {
			continue
		}
		for _, pi := range procs.Processors {
			newTrackOf[pi.Id] = t.ID
			switch {
			case strings.HasPrefix(pi.Name, prefixMeter):
				t.meterProc = pi.Id
				t.hidden = append(t.hidden, pi.Id)
				hp := e.hiddenParams(c, pi.Id)
				for ch := 0; ch < 2; ch++ {
					if id, ok := hp[fmt.Sprintf("level_%d", ch)]; ok {
						newMeterRefs[pkey{pi.Id, id}] = meterRef{track: t.ID, ch: ch}
					}
					if id, ok := hp[fmt.Sprintf("clip_%d", ch)]; ok {
						newMeterRefs[pkey{pi.Id, id}] = meterRef{track: t.ID, ch: ch, clip: true}
					}
				}
			case strings.HasPrefix(pi.Name, prefixReturn):
				t.returnProc, t.returnName = pi.Id, pi.Name
			case strings.HasPrefix(pi.Name, prefixSend):
				t.hidden = append(t.hidden, pi.Id)
				dest := ""
				if pid, err := c.Param.GetPropertyId(ctxT(), &rpc.PropertyIdRequest{Processor: &rpc.ProcessorIdentifier{Id: pi.Id}, PropertyName: "destination_name"}); err == nil && pid.Id != nil {
					if v, err := c.Param.GetPropertyValue(ctxT(), pid.Id); err == nil {
						dest = v.Value
					}
				}
				sends = append(sends, sendInfo{t, pi.Id, dest})
			default:
				e.mu.Lock()
				p := e.procCache[pi.Id]
				e.mu.Unlock()
				if p == nil || p.Name != pi.Name {
					p = e.loadProc(c, pi)
				}
				if p.UID == "" {
					unknownProcs = true
				}
				newCache[p.ID] = p
				t.Chain = append(t.Chain, p)
				for _, prm := range p.Params {
					newParams[pkey{p.ID, prm.ID}] = prm
				}
			}
		}
		// kind
		if t.Kind == "" {
			switch {
			case t.returnProc != 0:
				t.Kind = "bus"
			case kbd[t.ID] != nil:
				t.Kind = "midi"
			default:
				t.Kind = "audio"
				if tm := e.meta.Tracks[t.Name]; tm != nil && tm.Kind == "midi" {
					t.Kind = "midi"
				}
			}
		}
		t.Input, t.Output = e.routing(c, t, kbd[t.ID])
		newTracks[t.ID] = t
		switch t.Kind {
		case "pre":
			pre = t
		case "post":
			post = t
		case "bus":
			buses = append(buses, t)
		default:
			regular = append(regular, t)
		}
	}

	if unknownProcs {
		e.learnProcsFromSession(c, newCache)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	// resolve sends: destination_name is the bus's return processor name
	byReturn := map[string]*Track{}
	for _, b := range buses {
		byReturn[b.returnName] = b
	}
	for _, s := range sends {
		b := byReturn[s.dest]
		if b == nil {
			continue
		}
		e.mu.Unlock()
		gid := e.hiddenParams(c, s.proc)["gain"]
		e.mu.Lock()
		sm := e.meta.track(s.track.Name).sends(b.Name)
		written := float32(math.NaN())
		if old := oldTracks[s.track.ID]; old != nil {
			for _, os := range old.Sends {
				if os.proc == s.proc {
					written = os.written
				}
			}
		}
		s.track.Sends = append(s.track.Sends, &Send{Bus: b.ID, BusName: b.Label, DB: sm.DB, Post: sm.Post, proc: s.proc, gainParam: gid, written: written})
	}
	// metadata: kinds, colors, solo, user mute, macros
	order := func(ts []*Track) {
		sort.SliceStable(ts, func(i, j int) bool {
			a, b := e.meta.Tracks[ts[i].Name], e.meta.Tracks[ts[j].Name]
			if a == nil || b == nil {
				return a != nil
			}
			return a.Order < b.Order
		})
	}
	order(regular)
	order(buses)
	ci := 0
	for _, t := range append(append(append([]*Track{}, regular...), buses...), nonNil(pre, post)...) {
		tm := e.meta.track(t.Name)
		if tm.Kind != t.Kind {
			tm.Kind = t.Kind
			e.metaDirty = true
		}
		if tm.Color == "" {
			tm.Color = palette[(len(e.meta.Tracks)+ci)%len(palette)]
			if t.Kind == "bus" {
				tm.Color = palette[(6+ci)%len(palette)]
			}
			ci++
			e.metaDirty = true
		}
		t.Color, t.Solo = tm.Color, tm.Solo
		if t.Kind == "pre" || t.Kind == "post" {
			t.Color, t.Solo = "#8c95a5", false
			if t.Kind == "post" {
				t.Color = "#e7e9ee"
			}
		}
		if old := oldTracks[t.ID]; old != nil {
			t.Mute = old.Mute
		} else if e.soloActiveIn(newTracks) || tm.Mute {
			t.Mute = tm.Mute
		} else {
			t.Mute = t.sushiMute
		}
		for _, p := range t.Chain {
			p.Macros = macroIDs(p, tm.Macros[p.Name])
		}
		if slot, ok := e.slots[t.ID]; ok {
			t.Meter = slot
		} else if t.meterProc != 0 {
			t.Meter = len(e.slots)
			for e.slotUsed(t.Meter) {
				t.Meter++
			}
			e.slots[t.ID] = t.Meter
		}
		if t.meterProc == 0 {
			t.Meter = -1
		}
	}
	for id := range e.slots { // forget slots of deleted tracks
		if newTracks[id] == nil {
			delete(e.meters, e.slots[id])
			delete(e.slots, id)
		}
	}
	e.tracks, e.params, e.meterRefs, e.muteKeys, e.mixKeys, e.trackOf, e.procCache = newTracks, newParams, newMeterRefs, newMute, newMix, newTrackOf, newCache
	e.st.Tracks = append(regular, buses...)
	if e.st.Tracks == nil {
		e.st.Tracks = []*Track{}
	}
	e.st.Pre, e.st.Master = pre, post
	e.st.Project.List = listProjects(e.dataDir)
	e.sendsDirty, e.stateDirty = true, true
}

func nonNil(ts ...*Track) []*Track {
	var out []*Track
	for _, t := range ts {
		if t != nil {
			out = append(out, t)
		}
	}
	return out
}

func (e *Engine) soloActiveIn(ts map[int32]*Track) bool {
	for _, t := range ts {
		if tm := e.meta.Tracks[t.Name]; tm != nil && tm.Solo {
			return true
		}
	}
	return false
}

func (e *Engine) slotUsed(s int) bool {
	for _, v := range e.slots {
		if v == s {
			return true
		}
	}
	return false
}

func macroIDs(p *Proc, names []string) []int32 {
	ids := []int32{}
	if names != nil {
		for _, n := range names {
			for _, prm := range p.Params {
				if prm.Name == n {
					ids = append(ids, prm.ID)
				}
			}
		}
		return ids
	}
	for _, prm := range p.Params { // default: first three automatable float params
		if prm.Automatable && prm.Type == "float" && len(ids) < 3 {
			ids = append(ids, prm.ID)
		}
	}
	return ids
}

func (e *Engine) routing(c *sushi.Client, t *Track, kbd *rpc.MidiKbdConnection) (string, string) {
	chans := func(prefix string, conns []*rpc.AudioConnection) string {
		var ch []int
		for _, a := range conns {
			if !slices.Contains(ch, int(a.EngineChannel)) {
				ch = append(ch, int(a.EngineChannel))
			}
		}
		sort.Ints(ch)
		switch {
		case len(ch) == 0:
			return "none"
		case len(ch) == 1:
			return fmt.Sprintf("%s %d", prefix, ch[0]+1)
		default:
			return fmt.Sprintf("%s %d-%d", prefix, ch[0]+1, ch[len(ch)-1]+1)
		}
	}
	e.mu.Lock()
	ins, outs := e.st.Sushi.Inputs, e.st.Sushi.Outputs
	e.mu.Unlock()
	switch t.Kind {
	case "pre":
		return fmt.Sprintf("In 1-%d", max(ins, 1)), "all tracks"
	case "post":
		return "all tracks", fmt.Sprintf("Out 1-%d", max(outs, 1))
	}
	in := "none"
	if t.returnProc != 0 {
		in = "Sends"
	} else if kbd != nil {
		chn := "Omni"
		if c := kbd.GetChannel().GetChannel(); c != rpc.MidiChannel_MIDI_CH_OMNI {
			chn = fmt.Sprintf("ch %d", int(c))
		}
		in = fmt.Sprintf("MIDI port %d · %s", kbd.Port, chn)
	} else if r, err := c.Routing.GetInputConnectionsForTrack(ctxT(), &rpc.TrackIdentifier{Id: t.ID}); err == nil {
		in = chans("In", r.Connections)
	}
	out := "none"
	if r, err := c.Routing.GetOutputConnectionsForTrack(ctxT(), &rpc.TrackIdentifier{Id: t.ID}); err == nil {
		out = chans("Out", r.Connections)
	}
	return in, out
}

func (e *Engine) loadParam(c *sushi.Client, proc int32, pi *rpc.ParameterInfo) *Param {
	p := &Param{ID: pi.Id, Name: pi.Name, Label: pi.Label, Unit: pi.Unit, Automatable: pi.Automatable, Min: pi.MinDomainValue, Max: pi.MaxDomainValue}
	switch pi.GetType().GetType() {
	case rpc.ParameterType_BOOL:
		p.Type = "bool"
	case rpc.ParameterType_INT:
		p.Type = "int"
	default:
		p.Type = "float"
	}
	id := &rpc.ParameterIdentifier{ProcessorId: proc, ParameterId: pi.Id}
	if v, err := c.Param.GetParameterValue(ctxT(), id); err == nil {
		p.N = v.Value
	}
	if v, err := c.Param.GetParameterValueAsString(ctxT(), id); err == nil {
		p.Txt = v.Value
	}
	return p
}

func (e *Engine) hiddenParams(c *sushi.Client, proc int32) map[string]int32 {
	e.mu.Lock()
	hp, ok := e.hidParams[proc]
	e.mu.Unlock()
	if ok {
		return hp
	}
	hp = map[string]int32{}
	if ps, err := c.Param.GetProcessorParameters(ctxT(), &rpc.ProcessorIdentifier{Id: proc}); err == nil {
		for _, pi := range ps.Parameters {
			hp[pi.Name] = pi.Id
		}
	}
	e.mu.Lock()
	e.hidParams[proc] = hp
	e.mu.Unlock()
	return hp
}

func (e *Engine) loadProc(c *sushi.Client, pi *rpc.ProcessorInfo) *Proc {
	p := &Proc{ID: pi.Id, Name: pi.Name, Label: pi.Label, Program: -1, Programs: []string{}, Params: []*Param{}, Props: []*Prop{}, Role: catalog.AudioFx, Type: "unknown"}
	id := &rpc.ProcessorIdentifier{Id: pi.Id}
	if ps, err := c.Param.GetProcessorParameters(ctxT(), id); err == nil {
		for _, info := range ps.Parameters {
			p.Params = append(p.Params, e.loadParam(c, pi.Id, info))
		}
	}
	if b, err := c.Graph.GetProcessorBypassState(ctxT(), id); err == nil {
		p.Bypass = b.Value
	}
	if pi.ProgramCount > 0 {
		if pr, err := c.Program.GetProcessorPrograms(ctxT(), id); err == nil {
			for _, x := range pr.Programs {
				p.Programs = append(p.Programs, x.Name)
			}
		}
		if cur, err := c.Program.GetProcessorCurrentProgram(ctxT(), id); err == nil && sushi.Check("", cur.Status) == nil {
			p.Program = cur.Program
		}
	}
	if props, err := c.Param.GetProcessorProperties(ctxT(), id); err == nil {
		for _, pr := range props.Properties {
			v, _ := c.Param.GetPropertyValue(ctxT(), &rpc.PropertyIdentifier{ProcessorId: pi.Id, PropertyId: pr.Id})
			p.Props = append(p.Props, &Prop{ID: pr.Id, Name: pr.Name, Label: pr.Label, Value: v.GetValue()})
		}
	}
	e.mu.Lock()
	pm := e.meta.Procs[p.Name]
	e.mu.Unlock()
	if pm != nil {
		e.applyProcMeta(p, pm)
	}
	return p
}

func (e *Engine) applyProcMeta(p *Proc, pm *ProcMeta) {
	p.UID, p.Type, p.Path = pm.UID, pm.Type, pm.Path
	if pm.Role != "" {
		p.Role = catalog.Role(pm.Role)
	} else if ce, ok := e.cat.ByUID(pm.UID); ok {
		p.Role = ce.Role
	}
	if ce, ok := e.cat.ByUID(pm.UID); ok && p.Label == "" {
		p.Label = ce.Name
	}
}

// learnProcsFromSession fills uid/type/path for processors created outside the WebUI, using
// SaveSession (each TrackState lists PluginClass{name, uid, path, type}).
func (e *Engine) learnProcsFromSession(c *sushi.Client, procs map[int32]*Proc) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ss, err := c.Session.SaveSession(ctx, sushi.Void)
	if err != nil {
		return
	}
	byName := map[string]*rpc.PluginClass{}
	for _, ts := range ss.Tracks {
		for _, pc := range ts.Processors {
			byName[pc.Name] = pc
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range procs {
		if p.UID != "" {
			continue
		}
		pc := byName[p.Name]
		if pc == nil {
			continue
		}
		pm := &ProcMeta{UID: pc.Uid, Path: pc.Path, Type: pluginTypeName(pc.GetType().GetType())}
		if ce, ok := e.cat.ByUID(pc.Uid); ok {
			pm.Role = string(ce.Role)
		}
		e.meta.Procs[p.Name] = pm
		e.metaDirty = true
		e.applyProcMeta(p, pm)
	}
}

func pluginTypeName(t rpc.PluginType_Type) string {
	switch t {
	case rpc.PluginType_INTERNAL:
		return "internal"
	case rpc.PluginType_VST2X:
		return "vst2"
	case rpc.PluginType_VST3X:
		return "vst3"
	case rpc.PluginType_LV2:
		return "lv2"
	}
	return "unknown"
}

func pluginTypeEnum(s string) rpc.PluginType_Type {
	switch s {
	case "internal":
		return rpc.PluginType_INTERNAL
	case "vst2":
		return rpc.PluginType_VST2X
	case "vst3":
		return rpc.PluginType_VST3X
	case "lv2":
		return rpc.PluginType_LV2
	}
	return rpc.PluginType_DUMMY
}

// resetStaleMeta forgets the saved metadata (project name, colors, solo...) when Sushi came up with a
// different set of tracks than the metadata describes, e.g. Sushi was restarted from its default
// config after a project had been saved. The metadata is keyed by track names, so a matching set
// means Sushi is still running the same session (only the backend restarted).
func (e *Engine) resetStaleMeta(c *sushi.Client) {
	all, err := c.Graph.GetAllTracks(ctxT(), sushi.Void)
	if err != nil {
		return
	}
	fixed := map[string]bool{} // PRE and POST tracks: not part of the user's session
	live := map[string]bool{}
	for _, t := range all.Tracks {
		switch t.GetType().GetType() {
		case rpc.TrackType_PRE, rpc.TrackType_POST:
			fixed[t.Name] = true
		default:
			live[t.Name] = true
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	same := true
	for n := range e.meta.Tracks {
		if !fixed[n] && !live[n] {
			same = false
		}
	}
	for n := range live {
		if _, ok := e.meta.Tracks[n]; !ok {
			same = false
		}
	}
	if same {
		return
	}
	slog.Info("Sushi's tracks don't match the saved metadata; starting a new untitled project", "project", e.meta.Project)
	ui := e.meta.UI
	e.meta = newMeta()
	e.meta.UI = ui
	e.st.Project = ProjectInfo{Name: e.meta.Project, List: listProjects(e.dataDir)}
	e.metaDirty = true
}
