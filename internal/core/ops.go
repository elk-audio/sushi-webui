package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elk-audio/sushi-webui/internal/catalog"
	"github.com/elk-audio/sushi-webui/internal/sushi"
	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"google.golang.org/protobuf/proto"
)

// UserError is shown to the user as is.
type UserError struct{ Msg string }

func (e *UserError) Error() string { return e.Msg }

func userErr(format string, a ...any) error { return &UserError{fmt.Sprintf(format, a...)} }

// Command runs one browser command (PROTOCOL.md). Commands are serialized.
func (e *Engine) Command(op string, raw json.RawMessage) (map[string]any, error) {
	e.opMu.Lock()
	defer e.opMu.Unlock()
	if e.client() == nil {
		return nil, userErr("Sushi isn't connected")
	}
	var a struct {
		Kind        string   `json:"kind"`
		Name        string   `json:"name"`
		Channels    int      `json:"channels"`
		Input       int      `json:"input"`
		Output      int      `json:"output"`
		MidiPort    int      `json:"midiPort"`
		MidiChannel int      `json:"midiChannel"`
		Instrument  string   `json:"instrument"`
		Fx          string   `json:"fx"`
		SendFrom    *int32   `json:"sendFrom"`
		Track       int32    `json:"track"`
		Bus         int32    `json:"bus"`
		Proc        int32    `json:"proc"`
		Prop        int32    `json:"prop"`
		Value       string   `json:"value"`
		CatalogID   string   `json:"catalogId"`
		Section     string   `json:"section"`
		Index       int      `json:"index"`
		On          bool     `json:"on"`
		Post        bool     `json:"post"`
		Program     int32    `json:"program"`
		Params      []int32  `json:"params"`
		Color       string   `json:"color"`
		Playing     *bool    `json:"playing"`
		Tempo       *float32 `json:"tempo"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, userErr("invalid arguments: %v", err)
		}
	}
	var err error
	var newID int32 = -1
	structural := true
	switch op {
	case "addTrack":
		newID, err = e.addTrack(a.Kind, a.Name, a.Channels, a.Input, a.Output, a.MidiPort, a.MidiChannel, a.Instrument, a.Fx, a.SendFrom)
	case "removeTrack":
		err = e.removeTrack(a.Track)
	case "renameTrack":
		structural = false
		err = e.renameTrack(a.Track, a.Name)
	case "setColor":
		structural = false
		err = e.withTrack(a.Track, func(t *Track) { t.Color = a.Color; e.meta.track(t.Name).Color = a.Color })
	case "setMute":
		structural = false
		err = e.withTrack(a.Track, func(t *Track) { t.Mute = a.On; e.meta.track(t.Name).Mute = a.On })
		e.applyMutesLocked()
	case "setSolo":
		structural = false
		err = e.withTrack(a.Track, func(t *Track) { t.Solo = a.On; e.meta.track(t.Name).Solo = a.On })
		e.applyMutesLocked()
	case "addPlugin":
		newID, err = e.addPlugin(a.Track, a.CatalogID, catalog.Role(a.Section))
	case "removePlugin":
		err = e.removePlugin(a.Proc)
	case "movePlugin":
		err = e.movePlugin(a.Proc, a.Index)
	case "setBypass":
		structural = false
		err = e.setBypass(a.Proc, a.On)
	case "setProgram":
		structural = false
		err = e.setProgram(a.Proc, a.Program)
	case "setMacros":
		structural = false
		err = e.withProc(a.Proc, func(t *Track, p *Proc) {
			names := []string{}
			ids := []int32{}
			for _, id := range a.Params {
				for _, prm := range p.Params {
					if prm.ID == id && len(ids) < 6 {
						names, ids = append(names, prm.Name), append(ids, id)
					}
				}
			}
			p.Macros = ids
			e.meta.track(t.Name).Macros[p.Name] = names
		})
	case "setProperty":
		structural = false
		err = e.setProperty(a.Proc, a.Prop, a.Value)
	case "listWavs":
		return map[string]any{"files": e.listWavs()}, nil
	case "setWavFile":
		structural = false
		err = e.setWavFile(a.Proc, a.Value)
	case "addSend":
		err = e.addSend(a.Track, a.Bus)
	case "removeSend":
		err = e.removeSend(a.Track, a.Bus)
	case "setSendMode":
		structural = false
		err = e.withTrack(a.Track, func(t *Track) {
			for _, s := range t.Sends {
				if s.Bus == a.Bus {
					s.Post = a.Post
					e.meta.track(t.Name).sends(s.BusName).Post = a.Post
				}
			}
			e.sendsDirty = true
		})
	case "setAdvanced":
		structural = false
		e.mu.Lock()
		e.meta.UI.Advanced, e.st.UI.Advanced = a.On, a.On
		e.metaDirty = true
		e.mu.Unlock()
	case "transport":
		structural = false
		err = e.transport(a.Playing, a.Tempo)
	case "saveProject":
		structural = false
		err = e.saveProject(a.Name)
	case "loadProject":
		err = e.loadProject(a.Name)
	default:
		return nil, userErr("unknown command %q", op)
	}
	e.mu.Lock()
	e.stateDirty, e.metaDirty = true, true
	if op != "saveProject" && op != "loadProject" && op != "setAdvanced" {
		e.st.Project.Dirty = true
	}
	e.mu.Unlock()
	if structural {
		e.refreshLocked()
		e.applyMutesLocked()
	}
	if err != nil {
		slog.Warn("command failed", "op", op, "err", err)
		return nil, err
	}
	if newID >= 0 {
		return map[string]any{"id": newID}, nil
	}
	return nil, nil
}

// renameTrack changes the name shown for a track. Sushi has no rename call, so the Sushi-side name
// stays the same and the label is kept in the metadata (and saved with projects).
func (e *Engine) renameTrack(id int32, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return userErr("a track needs a name")
	}
	if len([]rune(name)) > 40 {
		return userErr("track names can have at most 40 characters")
	}
	err := e.withTrack(id, func(t *Track) {})
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	t := e.tracks[id]
	if t.Kind == "pre" || t.Kind == "post" {
		return userErr("%s can't be renamed", t.Label)
	}
	for _, o := range e.tracks {
		if o.ID != id && strings.EqualFold(o.Label, name) {
			return userErr("there is already a track called %q", name)
		}
	}
	tm := e.meta.track(t.Name)
	if name == t.Name {
		tm.Label = ""
	} else {
		tm.Label = name
	}
	t.Label = name
	e.stateDirty, e.metaDirty = true, true
	for _, o := range e.tracks { // sends show the bus name
		for _, s := range o.Sends {
			if s.Bus == id {
				s.BusName = name
			}
		}
	}
	return nil
}

func (e *Engine) withTrack(id int32, fn func(t *Track)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := e.tracks[id]
	if t == nil {
		return userErr("that track no longer exists")
	}
	fn(t)
	return nil
}

func (e *Engine) withProc(id int32, fn func(t *Track, p *Proc)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	t := e.tracks[e.trackOf[id]]
	if t == nil {
		return userErr("that plugin no longer exists")
	}
	for _, p := range t.Chain {
		if p.ID == id {
			fn(t, p)
			return nil
		}
	}
	return userErr("that plugin no longer exists")
}

// ---------------------------------------------------------------- setup

// setupLocked makes sure the WebUI's invariants hold on a fresh connection: a Master (POST) track
// exists and every track has a hidden meter as its last processor.
func (e *Engine) setupLocked() error {
	c := e.client()
	all, err := c.Graph.GetAllTracks(ctxT(), sushi.Void)
	if err != nil {
		return err
	}
	hasPost := false
	for _, t := range all.Tracks {
		if t.GetType().GetType() == rpc.TrackType_POST {
			hasPost = true
		}
	}
	if !hasPost {
		r, err := c.Graph.CreatePostTrack(ctxT(), &rpc.CreatePostTrackRequest{Name: "Master"})
		if err := c.Cmd("CreatePostTrack", r, err); err != nil {
			slog.Warn("couldn't create the Master track", "err", err)
		} else if _, err := sushi.WaitFor(2*time.Second, func() (int32, error) { return c.TrackID("Master") }); err == nil {
			all, _ = c.Graph.GetAllTracks(ctxT(), sushi.Void)
		}
	}
	for _, t := range all.Tracks {
		procs, err := c.Graph.GetTrackProcessors(ctxT(), &rpc.TrackIdentifier{Id: t.Id})
		if err != nil {
			continue
		}
		has := slices.ContainsFunc(procs.Processors, func(p *rpc.ProcessorInfo) bool { return strings.HasPrefix(p.Name, prefixMeter) })
		if !has {
			if _, err := e.createProc(t.Id, uniqueName(prefixMeter+t.Name), "sushi.testing.peakmeter", "", "internal", -1); err != nil {
				slog.Warn("couldn't add a meter", "track", t.Name, "err", err)
			}
		}
	}
	return nil
}

func uniqueName(base string) string { return fmt.Sprintf("%s#%04x", base, rand.IntN(0x10000)) }

// createProc creates a processor and returns its id. before < 0 appends to the back.
func (e *Engine) createProc(track int32, name, uid, path, typ string, before int32) (int32, error) {
	c := e.client()
	pos := &rpc.ProcessorPosition{AddToBack: before < 0}
	if before >= 0 {
		pos.BeforeProcessor = &rpc.ProcessorIdentifier{Id: before}
	}
	r, err := c.Graph.CreateProcessorOnTrack(ctxT(), &rpc.CreateProcessorRequest{
		Name: name, Uid: uid, Path: path, Type: &rpc.PluginType{Type: pluginTypeEnum(typ)},
		Track: &rpc.TrackIdentifier{Id: track}, Position: pos,
	})
	if err := c.Cmd("CreateProcessorOnTrack", r, err); err != nil {
		return 0, err
	}
	return sushi.WaitFor(2*time.Second, func() (int32, error) { return c.ProcessorID(name) })
}

// ---------------------------------------------------------------- tracks

func (e *Engine) uniqueTrackName(name string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	taken := map[string]bool{}
	for _, t := range e.tracks {
		taken[strings.ToLower(t.Name)] = true
	}
	if !taken[strings.ToLower(name)] {
		return name
	}
	for i := 2; ; i++ {
		n := fmt.Sprintf("%s %d", name, i)
		if !taken[strings.ToLower(n)] {
			return n
		}
	}
}

func (e *Engine) addTrack(kind, name string, channels, input, output, midiPort, midiCh int, instrument, fx string, sendFrom *int32) (int32, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = map[string]string{"audio": "Audio", "midi": "MIDI", "bus": "Bus"}[kind]
	}
	if strings.HasPrefix(name, "__") {
		return -1, userErr("track names can't start with __")
	}
	if kind != "audio" && kind != "midi" && kind != "bus" {
		return -1, userErr("unknown track kind %q", kind)
	}
	name = e.uniqueTrackName(name)
	if kind != "audio" || channels < 1 || channels > 2 {
		if kind == "audio" && channels == 0 {
			channels = 1
		} else if kind != "audio" {
			channels = 2
		}
	}
	c := e.client()
	r, err := c.Graph.CreateTrack(ctxT(), &rpc.CreateTrackRequest{Name: name, Channels: 2})
	if err := c.Cmd("CreateTrack", r, err); err != nil {
		return -1, userErr("Sushi couldn't create the track: %v", err)
	}
	id, err := sushi.WaitFor(2*time.Second, func() (int32, error) { return c.TrackID(name) })
	if err != nil {
		return -1, userErr("Sushi didn't report the new track: %v", err)
	}
	tid := &rpc.TrackIdentifier{Id: id}
	e.mu.Lock()
	tm := e.meta.track(name)
	tm.Kind = kind
	tm.Order = len(e.meta.Tracks) + 100
	e.mu.Unlock()

	switch kind {
	case "audio":
		// Mono inputs become dual-mono 2-channel tracks: Sushi 1.4 exits when a stereo-only VST3 is
		// added to a 1-channel track ("Failed to set a valid channel arrangement").
		for ch := 0; ch < 2; ch++ {
			src := input + ch
			if channels == 1 {
				src = input
			}
			r, err := c.Routing.ConnectInputChannelToTrack(ctxT(), &rpc.AudioConnection{Track: tid, TrackChannel: int32(ch), EngineChannel: int32(src)})
			if err := c.Cmd("ConnectInputChannelToTrack", r, err); err != nil {
				slog.Warn("input connection failed", "track", name, "err", err)
			}
		}
	case "midi":
		mc := rpc.MidiChannel_MIDI_CH_OMNI
		if midiCh >= 1 && midiCh <= 16 {
			mc = rpc.MidiChannel_Channel(midiCh)
		}
		r, err := c.Midi.ConnectKbdInputToTrack(ctxT(), &rpc.MidiKbdConnection{Track: tid, Channel: &rpc.MidiChannel{Channel: mc}, Port: int32(midiPort)})
		if err := c.Cmd("ConnectKbdInputToTrack", r, err); err != nil {
			slog.Warn("MIDI connection failed", "track", name, "err", err)
		}
	case "bus":
		if _, err := e.createProc(id, uniqueName(prefixReturn+name), "sushi.testing.return", "", "internal", -1); err != nil {
			return id, userErr("Sushi couldn't create the bus return: %v", err)
		}
	}
	for ch := 0; ch < 2; ch++ {
		r, err := c.Routing.ConnectOutputChannelFromTrack(ctxT(), &rpc.AudioConnection{Track: tid, TrackChannel: int32(ch), EngineChannel: int32(output + ch)})
		if err := c.Cmd("ConnectOutputChannelFromTrack", r, err); err != nil {
			slog.Warn("output connection failed", "track", name, "ch", ch, "err", err)
		}
	}
	meter, err := e.createProc(id, uniqueName(prefixMeter+name), "sushi.testing.peakmeter", "", "internal", -1)
	if err != nil {
		slog.Warn("couldn't add a meter", "track", name, "err", err)
		meter = -1
	}
	first := instrument
	if kind == "bus" {
		first = fx
	}
	if first != "" {
		if ce, ok := e.cat.Get(first); ok {
			if _, err := e.createUserProc(id, ce, meter); err != nil {
				return id, userErr("created %s, but Sushi couldn't load %s: %v", name, ce.Name, err)
			}
		}
	}
	if kind == "bus" && sendFrom != nil {
		e.refreshLocked()
		return id, e.addSend(*sendFrom, id)
	}
	return id, nil
}

func (e *Engine) removeTrack(id int32) error {
	e.mu.Lock()
	t := e.tracks[id]
	var senders []sendKey
	if t != nil && t.Kind == "bus" {
		for _, o := range e.tracks {
			for _, s := range o.Sends {
				if s.Bus == id {
					senders = append(senders, sendKey{o.ID, id})
				}
			}
		}
	}
	e.mu.Unlock()
	if t == nil {
		return userErr("that track no longer exists")
	}
	if t.Kind == "post" {
		return userErr("the Master track can't be deleted")
	}
	for _, s := range senders {
		if err := e.removeSend(s.track, s.bus); err != nil {
			return err
		}
	}
	c := e.client()
	procs, err := c.Graph.GetTrackProcessors(ctxT(), &rpc.TrackIdentifier{Id: id})
	if err == nil {
		for _, p := range procs.Processors {
			r, err := c.Graph.DeleteProcessorFromTrack(ctxT(), &rpc.DeleteProcessorRequest{Processor: &rpc.ProcessorIdentifier{Id: p.Id}, Track: &rpc.TrackIdentifier{Id: id}})
			if err := c.Cmd("DeleteProcessorFromTrack", r, err); err != nil {
				slog.Warn("couldn't delete processor", "proc", p.Name, "err", err)
			}
		}
	}
	r, err := c.Graph.DeleteTrack(ctxT(), &rpc.TrackIdentifier{Id: id})
	if err := c.Cmd("DeleteTrack", r, err); err != nil {
		return userErr("Sushi couldn't delete %s: %v", t.Name, err)
	}
	e.mu.Lock()
	delete(e.meta.Tracks, t.Name)
	for _, o := range e.meta.Tracks {
		delete(o.Sends, t.Name)
	}
	e.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------- plugins

func sectionIndex(r catalog.Role) int {
	switch r {
	case catalog.MidiFx:
		return 0
	case catalog.Instrument:
		return 1
	}
	return 2
}

// insertBefore returns the processor the new one goes before so the chain stays ordered by section
// and hidden sends/meter stay last. -1 means append.
func (e *Engine) insertBefore(t *Track, role catalog.Role) int32 {
	for _, p := range t.Chain {
		if sectionIndex(p.Role) > sectionIndex(role) {
			return p.ID
		}
	}
	if len(t.hidden) > 0 {
		return t.hidden[0]
	}
	return -1
}

func (e *Engine) createUserProc(track int32, ce *catalog.Entry, before int32) (int32, error) {
	e.mu.Lock()
	taken := map[string]bool{}
	for _, t := range e.tracks {
		for _, p := range t.Chain {
			taken[p.Name] = true
		}
	}
	e.mu.Unlock()
	name := ce.Name
	for i := 2; taken[name]; i++ {
		name = fmt.Sprintf("%s %d", ce.Name, i)
	}
	if _, err := e.client().ProcessorID(name); err == nil { // also taken by something we don't show
		name = uniqueName(ce.Name)
	}
	id, err := e.createProc(track, name, ce.UID, ce.Path, ce.Type, before)
	if err != nil {
		return 0, err
	}
	e.mu.Lock()
	e.meta.Procs[name] = &ProcMeta{UID: ce.UID, Type: ce.Type, Path: ce.Path, Role: string(ce.Role)}
	e.mu.Unlock()
	return id, nil
}

func (e *Engine) addPlugin(track int32, catalogID string, section catalog.Role) (int32, error) {
	ce, ok := e.cat.Get(catalogID)
	if !ok {
		return -1, userErr("unknown plugin %q", catalogID)
	}
	e.mu.Lock()
	t := e.tracks[track]
	var before int32 = -1
	if t != nil {
		if section == "" {
			section = ce.Role
		}
		if t.Kind != "midi" && section != catalog.AudioFx {
			e.mu.Unlock()
			return -1, userErr("%s tracks only take audio effects", t.Kind)
		}
		if section == catalog.Instrument && slices.ContainsFunc(t.Chain, func(p *Proc) bool { return p.Role == catalog.Instrument }) {
			e.mu.Unlock()
			return -1, userErr("%s already has an instrument; remove it first", t.Name)
		}
		before = e.insertBefore(t, section)
	}
	e.mu.Unlock()
	if t == nil {
		return -1, userErr("that track no longer exists")
	}
	id, err := e.createUserProc(track, ce, before)
	if err != nil {
		return -1, userErr("Sushi couldn't load %s: %v", ce.Name, err)
	}
	return id, nil
}

func (e *Engine) removePlugin(id int32) error {
	e.mu.Lock()
	tid := e.trackOf[id]
	e.mu.Unlock()
	c := e.client()
	r, err := c.Graph.DeleteProcessorFromTrack(ctxT(), &rpc.DeleteProcessorRequest{Processor: &rpc.ProcessorIdentifier{Id: id}, Track: &rpc.TrackIdentifier{Id: tid}})
	if err := c.Cmd("DeleteProcessorFromTrack", r, err); err != nil {
		return userErr("Sushi couldn't remove the plugin: %v", err)
	}
	return nil
}

func (e *Engine) movePlugin(id int32, index int) error {
	e.mu.Lock()
	t := e.tracks[e.trackOf[id]]
	if t == nil {
		e.mu.Unlock()
		return userErr("that plugin no longer exists")
	}
	var rest []*Proc
	for _, p := range t.Chain {
		if p.ID != id {
			rest = append(rest, p)
		}
	}
	var before int32 = -1
	if index >= 0 && index < len(rest) {
		before = rest[index].ID
	} else if len(t.hidden) > 0 {
		before = t.hidden[0]
	}
	tid := t.ID
	e.mu.Unlock()
	pos := &rpc.ProcessorPosition{AddToBack: before < 0}
	if before >= 0 {
		pos.BeforeProcessor = &rpc.ProcessorIdentifier{Id: before}
	}
	c := e.client()
	r, err := c.Graph.MoveProcessorOnTrack(ctxT(), &rpc.MoveProcessorRequest{
		Processor: &rpc.ProcessorIdentifier{Id: id}, SourceTrack: &rpc.TrackIdentifier{Id: tid}, DestTrack: &rpc.TrackIdentifier{Id: tid}, Position: pos,
	})
	if err := c.Cmd("MoveProcessorOnTrack", r, err); err != nil {
		return userErr("Sushi couldn't move the plugin: %v", err)
	}
	return nil
}

func (e *Engine) setBypass(id int32, on bool) error {
	c := e.client()
	r, err := c.Graph.SetProcessorBypassState(ctxT(), &rpc.ProcessorBypassStateSetRequest{Processor: &rpc.ProcessorIdentifier{Id: id}, Value: on})
	if err := c.Cmd("SetProcessorBypassState", r, err); err != nil {
		return userErr("Sushi couldn't change bypass: %v", err)
	}
	// Sushi has no bypass notification; update the mirror directly.
	return e.withProc(id, func(_ *Track, p *Proc) { p.Bypass = on })
}

func (e *Engine) setProgram(id, program int32) error {
	c := e.client()
	r, err := c.Program.SetProcessorProgram(ctxT(), &rpc.ProcessorProgramSetRequest{Processor: &rpc.ProcessorIdentifier{Id: id}, Program: &rpc.ProgramIdentifier{Program: program}})
	if err := c.Cmd("SetProcessorProgram", r, err); err != nil {
		return userErr("Sushi couldn't change the program: %v", err)
	}
	return e.withProc(id, func(_ *Track, p *Proc) { p.Program = program })
}

func (e *Engine) setProperty(proc, prop int32, value string) error {
	c := e.client()
	r, err := c.Param.SetPropertyValue(ctxT(), &rpc.PropertyValue{Property: &rpc.PropertyIdentifier{ProcessorId: proc, PropertyId: prop}, Value: value})
	if err := c.Cmd("SetPropertyValue", r, err); err != nil {
		return userErr("Sushi couldn't set the property: %v", err)
	}
	return e.withProc(proc, func(_ *Track, p *Proc) {
		for _, pr := range p.Props {
			if pr.ID == prop {
				pr.Value = value
			}
		}
	})
}

// ---------------------------------------------------------------- sends (ARCHITECTURE.md §4.7)

func (e *Engine) addSend(track, bus int32) error {
	e.mu.Lock()
	t, b := e.tracks[track], e.tracks[bus]
	if t == nil || b == nil || b.Kind != "bus" || (t.Kind != "audio" && t.Kind != "midi") {
		e.mu.Unlock()
		return userErr("sends go from an audio or MIDI track to a bus")
	}
	for _, s := range t.Sends {
		if s.Bus == bus {
			e.mu.Unlock()
			return userErr("%s already sends to %s", t.Name, b.Name)
		}
	}
	before := t.meterProc
	if before == 0 {
		before = -1
	}
	tName, bName, ret, ch := t.Name, b.Name, b.returnName, t.Channels
	e.mu.Unlock()

	c := e.client()
	id, err := e.createProc(track, uniqueName(prefixSend+tName+"__"+bName), "sushi.testing.send", "", "internal", before)
	if err != nil {
		return userErr("Sushi couldn't create the send: %v", err)
	}
	pid, err := c.Param.GetPropertyId(ctxT(), &rpc.PropertyIdRequest{Processor: &rpc.ProcessorIdentifier{Id: id}, PropertyName: "destination_name"})
	if err == nil && pid.Id != nil {
		r, err := c.Param.SetPropertyValue(ctxT(), &rpc.PropertyValue{Property: pid.Id, Value: ret})
		if err := c.Cmd("SetPropertyValue", r, err); err != nil {
			return userErr("Sushi couldn't route the send: %v", err)
		}
	}
	hp := e.hiddenParams(c, id)
	if cc, ok := hp["channel_count"]; ok && ch == 1 { // mono track: send one channel
		_, _ = c.Param.SetParameterValue(ctxT(), &rpc.ParameterValue{Parameter: &rpc.ParameterIdentifier{ProcessorId: id, ParameterId: cc}, Value: 1.0 / 16})
	}
	if g, ok := hp["gain"]; ok { // start silent; flushWrites sets the real level
		_, _ = c.Param.SetParameterValue(ctxT(), &rpc.ParameterValue{Parameter: &rpc.ParameterIdentifier{ProcessorId: id, ParameterId: g}, Value: 0})
	}
	e.mu.Lock()
	sm := e.meta.track(tName).sends(bName)
	sm.DB, sm.Post = -12, true
	e.mu.Unlock()
	return nil
}

func (e *Engine) removeSend(track, bus int32) error {
	e.mu.Lock()
	t := e.tracks[track]
	var proc int32
	var busName string
	if t != nil {
		for _, s := range t.Sends {
			if s.Bus == bus {
				proc, busName = s.proc, s.BusName
			}
		}
	}
	e.mu.Unlock()
	if proc == 0 {
		return userErr("that send no longer exists")
	}
	c := e.client()
	r, err := c.Graph.DeleteProcessorFromTrack(ctxT(), &rpc.DeleteProcessorRequest{Processor: &rpc.ProcessorIdentifier{Id: proc}, Track: &rpc.TrackIdentifier{Id: track}})
	if err := c.Cmd("DeleteProcessorFromTrack", r, err); err != nil {
		return userErr("Sushi couldn't remove the send: %v", err)
	}
	e.mu.Lock()
	delete(e.meta.track(t.Name).Sends, busName)
	e.mu.Unlock()
	return nil
}

// ---------------------------------------------------------------- mute / solo

// applyMutesLocked writes each track's effective mute to Sushi where it differs. Caller holds opMu.
func (e *Engine) applyMutesLocked() {
	c := e.client()
	if c == nil {
		return
	}
	type w struct {
		track, param int32
		on           bool
	}
	var writes []w
	e.mu.Lock()
	solo := e.soloActive()
	for _, t := range e.tracks {
		want := e.effectiveMute(t, solo)
		if want != t.sushiMute {
			writes = append(writes, w{t.ID, t.muteParam, want})
			t.sushiMute = want
		}
	}
	e.sendsDirty, e.stateDirty = true, true
	e.mu.Unlock()
	for _, x := range writes {
		v := float32(0)
		if x.on {
			v = 1
		}
		if _, err := c.Param.SetParameterValue(ctxT(), &rpc.ParameterValue{Parameter: &rpc.ParameterIdentifier{ProcessorId: x.track, ParameterId: x.param}, Value: v}); err != nil {
			slog.Warn("couldn't set mute", "track", x.track, "err", err)
		}
	}
}

// ---------------------------------------------------------------- transport

func (e *Engine) transport(playing *bool, tempo *float32) error {
	c := e.client()
	if playing != nil {
		m := rpc.PlayingMode_STOPPED
		if *playing {
			m = rpc.PlayingMode_PLAYING
		}
		r, err := c.Transport.SetPlayingMode(ctxT(), &rpc.PlayingMode{Mode: m})
		if err := c.Cmd("SetPlayingMode", r, err); err != nil {
			return userErr("Sushi couldn't change the transport: %v", err)
		}
	}
	if tempo != nil {
		r, err := c.Transport.SetTempo(ctxT(), &rpc.GenericFloatValue{Value: clampF(*tempo, 20, 300)})
		if err := c.Cmd("SetTempo", r, err); err != nil {
			return userErr("Sushi couldn't change the tempo: %v", err)
		}
	}
	return nil
}

// ---------------------------------------------------------------- projects (ARCHITECTURE.md §4.11)

func (e *Engine) saveProject(name string) error {
	e.mu.Lock()
	if strings.TrimSpace(name) == "" {
		name = e.meta.Project
	}
	e.mu.Unlock()
	dir, err := projectDir(e.dataDir, name)
	if err != nil {
		return &UserError{err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ss, err := e.client().Session.SaveSession(ctx, sushi.Void)
	if err != nil {
		return userErr("Sushi couldn't save the session: %v", err)
	}
	raw, err := proto.Marshal(ss)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return userErr("couldn't create %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.pb"), raw, 0o644); err != nil {
		return userErr("couldn't write the project: %v", err)
	}
	e.mu.Lock()
	e.meta.Project = strings.TrimSuffix(filepath.Base(dir), ".sushiproj")
	err = e.meta.save(filepath.Join(dir, "meta.json"))
	e.st.Project = ProjectInfo{Name: e.meta.Project, Dirty: false, List: listProjects(e.dataDir)}
	e.mu.Unlock()
	return err
}

func (e *Engine) loadProject(name string) error {
	dir, err := projectDir(e.dataDir, name)
	if err != nil {
		return &UserError{err.Error()}
	}
	raw, err := os.ReadFile(filepath.Join(dir, "session.pb"))
	if err != nil {
		return userErr("project %q not found", name)
	}
	ss := &rpc.SessionState{}
	if err := proto.Unmarshal(raw, ss); err != nil {
		return userErr("project %q is damaged: %v", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := e.client()
	r, err := c.Session.RestoreSession(ctx, ss)
	if err := c.Cmd("RestoreSession", r, err); err != nil {
		return userErr("Sushi couldn't restore %q: %v", name, err)
	}
	m := loadMeta(filepath.Join(dir, "meta.json"))
	m.Project = name
	e.mu.Lock()
	e.meta = m
	e.st.UI = m.UI
	e.st.Project = ProjectInfo{Name: name, List: listProjects(e.dataDir)}
	e.procCache = map[int32]*Proc{}
	e.tracks = map[int32]*Track{}
	e.mu.Unlock()
	time.Sleep(200 * time.Millisecond) // let Sushi finish rebuilding the graph
	if err := e.setupLocked(); err != nil {
		return err
	}
	return nil
}
