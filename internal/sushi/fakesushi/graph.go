package fakesushi

import (
	"context"
	"slices"
	"strings"
	"time"

	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"google.golang.org/protobuf/proto"
)

type procRec struct {
	id   int32
	name string
	uid  string
}

type trackRec struct {
	id    int32
	name  string
	ch    int32
	typ   rpc.TrackType_Type
	procs []*procRec
}

// Parameter ids the fake exposes: tracks have gain/pan/mute, peak meters level_0..1 / clip_0..1.
const (
	TrackGain = 0
	TrackPan  = 1
	TrackMute = 2
)

// MeterParam returns the parameter id of a peakmeter output ("level_0", "clip_1", ...).
func MeterParam(name string) int32 {
	return map[string]int32{"level_0": 0, "level_1": 1, "clip_0": 2, "clip_1": 3}[name]
}

// AddTrack creates a track synchronously (test setup, no notifications). It returns its id.
func (f *Fake) AddTrack(name string, typ rpc.TrackType_Type) int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addTrackLocked(name, 2, typ)
}

func (f *Fake) addTrackLocked(name string, ch int32, typ rpc.TrackType_Type) int32 {
	id := f.nextID
	f.nextID++
	f.tracks = append(f.tracks, &trackRec{id: id, name: name, ch: ch, typ: typ})
	return id
}

// ProcessorID returns the id of the processor with that name, or -1.
func (f *Fake) ProcessorID(name string) int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tracks {
		for _, p := range t.procs {
			if p.name == name {
				return p.id
			}
		}
	}
	return -1
}

// ProcessorNames lists the processors on a track by name, in chain order.
func (f *Fake) ProcessorNames(track string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, t := range f.tracks {
		if t.name == track {
			for _, p := range t.procs {
				out = append(out, p.name)
			}
		}
	}
	return out
}

type graph struct {
	rpc.UnimplementedAudioGraphControllerServer
	f *Fake
}

func (g *graph) GetAllTracks(context.Context, *rpc.GenericVoidValue) (*rpc.TrackInfoList, error) {
	g.f.mu.Lock()
	defer g.f.mu.Unlock()
	l := &rpc.TrackInfoList{}
	for _, t := range g.f.tracks {
		ti := &rpc.TrackInfo{Id: t.id, Name: t.name, Label: t.name, Channels: t.ch, Buses: 1, Type: &rpc.TrackType{Type: t.typ}}
		for _, p := range t.procs {
			ti.Processors = append(ti.Processors, &rpc.ProcessorIdentifier{Id: p.id})
		}
		l.Tracks = append(l.Tracks, ti)
	}
	return l, nil
}

func (g *graph) GetTrackId(_ context.Context, in *rpc.GenericStringValue) (*rpc.TrackIdentifierResponse, error) {
	g.f.mu.Lock()
	defer g.f.mu.Unlock()
	for _, t := range g.f.tracks {
		if t.name == in.Value {
			return &rpc.TrackIdentifierResponse{Status: ok(), Id: t.id}, nil
		}
	}
	return &rpc.TrackIdentifierResponse{Status: &rpc.CommandStatus{Status: rpc.CommandStatus_NOT_FOUND}}, nil
}

func (g *graph) GetProcessorId(_ context.Context, in *rpc.GenericStringValue) (*rpc.ProcessorIdentifierResponse, error) {
	g.f.mu.Lock()
	defer g.f.mu.Unlock()
	for _, t := range g.f.tracks {
		for _, p := range t.procs {
			if p.name == in.Value {
				return &rpc.ProcessorIdentifierResponse{Status: ok(), Id: p.id}, nil
			}
		}
	}
	return &rpc.ProcessorIdentifierResponse{Status: &rpc.CommandStatus{Status: rpc.CommandStatus_NOT_FOUND}}, nil
}

func (g *graph) GetTrackProcessors(_ context.Context, in *rpc.TrackIdentifier) (*rpc.ProcessorInfoListResponse, error) {
	g.f.mu.Lock()
	defer g.f.mu.Unlock()
	for _, t := range g.f.tracks {
		if t.id == in.Id {
			r := &rpc.ProcessorInfoListResponse{Status: ok()}
			for _, p := range t.procs {
				r.Processors = append(r.Processors, &rpc.ProcessorInfo{Id: p.id, Name: p.name, Label: p.name, ParameterCount: 4})
			}
			return r, nil
		}
	}
	return &rpc.ProcessorInfoListResponse{Status: &rpc.CommandStatus{Status: rpc.CommandStatus_NOT_FOUND}}, nil
}

func (g *graph) GetProcessorBypassState(context.Context, *rpc.ProcessorIdentifier) (*rpc.BoolResponse, error) {
	return &rpc.BoolResponse{Status: ok()}, nil
}

// async runs the mutation after AsyncLag and then reports completion, like Sushi's event loop.
func (g *graph) async(mutate func() []proto.Message) *rpc.CommandResponse {
	g.f.mu.Lock()
	g.f.nextReq++
	req := g.f.nextReq
	g.f.mu.Unlock()
	go func() {
		time.Sleep(g.f.AsyncLag)
		for _, m := range mutate() {
			g.f.Emit(m)
		}
		g.f.Emit(&rpc.AsyncCommandResponse{Status: ok(), RequestId: req})
	}()
	return &rpc.CommandResponse{Status: &rpc.CommandStatus{Status: rpc.CommandStatus_ASYNC_RESPONSE}, Id: req}
}

func (g *graph) createTrack(name string, ch int32, typ rpc.TrackType_Type) *rpc.CommandResponse {
	return g.async(func() []proto.Message {
		g.f.mu.Lock()
		id := g.f.addTrackLocked(name, ch, typ)
		g.f.mu.Unlock()
		return []proto.Message{&rpc.TrackUpdate{Action: rpc.TrackUpdate_TRACK_ADDED, Track: &rpc.TrackIdentifier{Id: id}}}
	})
}

func (g *graph) CreateTrack(_ context.Context, in *rpc.CreateTrackRequest) (*rpc.CommandResponse, error) {
	return g.createTrack(in.Name, in.Channels, rpc.TrackType_REGULAR), nil
}

func (g *graph) CreatePostTrack(_ context.Context, in *rpc.CreatePostTrackRequest) (*rpc.CommandResponse, error) {
	return g.createTrack(in.Name, 2, rpc.TrackType_POST), nil
}

func notFound() *rpc.CommandResponse {
	return &rpc.CommandResponse{Status: &rpc.CommandStatus{Status: rpc.CommandStatus_NOT_FOUND}}
}

func (f *Fake) trackByID(id int32) *trackRec {
	for _, t := range f.tracks {
		if t.id == id {
			return t
		}
	}
	return nil
}

func (t *trackRec) indexOf(id int32) int {
	for i, p := range t.procs {
		if p.id == id {
			return i
		}
	}
	return -1
}

// insertIndex resolves a ProcessorPosition on t to a slot in t.procs, or -1 when "before" isn't on the track.
func (t *trackRec) insertIndex(pos *rpc.ProcessorPosition) int {
	if pos == nil || pos.AddToBack || pos.BeforeProcessor == nil {
		return len(t.procs)
	}
	return t.indexOf(pos.BeforeProcessor.Id)
}

func procUpdate(a rpc.ProcessorUpdate_Action, proc, track int32) proto.Message {
	return &rpc.ProcessorUpdate{Action: a, Processor: &rpc.ProcessorIdentifier{Id: proc}, ParentTrack: &rpc.TrackIdentifier{Id: track}}
}

func (g *graph) CreateProcessorOnTrack(_ context.Context, in *rpc.CreateProcessorRequest) (*rpc.CommandResponse, error) {
	return g.async(func() []proto.Message {
		g.f.mu.Lock()
		defer g.f.mu.Unlock()
		t := g.f.trackByID(in.Track.GetId())
		if t == nil {
			return nil
		}
		at := t.insertIndex(in.Position)
		if at < 0 {
			return nil
		}
		id := g.f.nextID
		g.f.nextID++
		t.procs = slices.Insert(t.procs, at, &procRec{id: id, name: in.Name, uid: in.Uid})
		return []proto.Message{procUpdate(rpc.ProcessorUpdate_PROCESSOR_ADDED, id, t.id)}
	}), nil
}

func (g *graph) DeleteProcessorFromTrack(_ context.Context, in *rpc.DeleteProcessorRequest) (*rpc.CommandResponse, error) {
	g.f.mu.Lock()
	t := g.f.trackByID(in.Track.GetId())
	missing := t == nil || t.indexOf(in.Processor.GetId()) < 0
	g.f.mu.Unlock()
	if missing {
		return notFound(), nil
	}
	return g.async(func() []proto.Message {
		g.f.mu.Lock()
		defer g.f.mu.Unlock()
		i := t.indexOf(in.Processor.Id)
		if i < 0 {
			return nil
		}
		t.procs = slices.Delete(t.procs, i, i+1)
		return []proto.Message{procUpdate(rpc.ProcessorUpdate_PROCESSOR_DELETED, in.Processor.Id, t.id)}
	}), nil
}

// MoveProcessorOnTrack mirrors Sushi: the processor keeps its id, and the notifications are
// DELETED then ADDED. Cross-track moves are supported.
func (g *graph) MoveProcessorOnTrack(_ context.Context, in *rpc.MoveProcessorRequest) (*rpc.CommandResponse, error) {
	g.f.mu.Lock()
	src, dst := g.f.trackByID(in.SourceTrack.GetId()), g.f.trackByID(in.DestTrack.GetId())
	missing := src == nil || dst == nil || src.indexOf(in.Processor.GetId()) < 0
	g.f.mu.Unlock()
	if missing {
		return notFound(), nil
	}
	return g.async(func() []proto.Message {
		g.f.mu.Lock()
		defer g.f.mu.Unlock()
		i := src.indexOf(in.Processor.Id)
		if i < 0 {
			return nil
		}
		p := src.procs[i]
		src.procs = slices.Delete(src.procs, i, i+1)
		at := dst.insertIndex(in.Position)
		if at < 0 { // Sushi puts the processor back where it was
			src.procs = slices.Insert(src.procs, i, p)
			return nil
		}
		dst.procs = slices.Insert(dst.procs, at, p)
		return []proto.Message{
			procUpdate(rpc.ProcessorUpdate_PROCESSOR_DELETED, p.id, src.id),
			procUpdate(rpc.ProcessorUpdate_PROCESSOR_ADDED, p.id, dst.id),
		}
	}), nil
}

func (g *graph) DeleteTrack(_ context.Context, in *rpc.TrackIdentifier) (*rpc.CommandResponse, error) {
	g.f.mu.Lock()
	missing := g.f.trackByID(in.Id) == nil
	g.f.mu.Unlock()
	if missing {
		return notFound(), nil
	}
	return g.async(func() []proto.Message {
		g.f.mu.Lock()
		defer g.f.mu.Unlock()
		g.f.tracks = slices.DeleteFunc(g.f.tracks, func(t *trackRec) bool { return t.id == in.Id })
		return []proto.Message{&rpc.TrackUpdate{Action: rpc.TrackUpdate_TRACK_DELETED, Track: &rpc.TrackIdentifier{Id: in.Id}}}
	}), nil
}

type param struct {
	rpc.UnimplementedParameterControllerServer
	f *Fake
}

func pinfo(id int32, name string) *rpc.ParameterInfo {
	return &rpc.ParameterInfo{Id: id, Name: name, Label: name, Type: &rpc.ParameterType{Type: rpc.ParameterType_FLOAT}, MaxDomainValue: 1}
}

func (p *param) GetTrackParameters(_ context.Context, in *rpc.TrackIdentifier) (*rpc.ParameterInfoListResponse, error) {
	return &rpc.ParameterInfoListResponse{Status: ok(), Parameters: []*rpc.ParameterInfo{
		pinfo(TrackGain, "gain"), pinfo(TrackPan, "pan"), pinfo(TrackMute, "mute")}}, nil
}

func (p *param) GetProcessorParameters(_ context.Context, in *rpc.ProcessorIdentifier) (*rpc.ParameterInfoListResponse, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	for _, t := range p.f.tracks {
		for _, pr := range t.procs {
			if pr.id == in.Id && strings.HasPrefix(pr.name, "__meter__") {
				return &rpc.ParameterInfoListResponse{Status: ok(), Parameters: []*rpc.ParameterInfo{
					pinfo(0, "level_0"), pinfo(1, "level_1"), pinfo(2, "clip_0"), pinfo(3, "clip_1")}}, nil
			}
		}
	}
	return &rpc.ParameterInfoListResponse{Status: ok()}, nil
}

func (p *param) GetProcessorProperties(context.Context, *rpc.ProcessorIdentifier) (*rpc.PropertyInfoListResponse, error) {
	return &rpc.PropertyInfoListResponse{Status: ok()}, nil
}

func (p *param) GetParameterValue(_ context.Context, in *rpc.ParameterIdentifier) (*rpc.FloatResponse, error) {
	p.f.mu.Lock()
	defer p.f.mu.Unlock()
	return &rpc.FloatResponse{Status: ok(), Value: p.f.params[[2]int32{in.ProcessorId, in.ParameterId}]}, nil
}

func (p *param) GetParameterValueAsString(_ context.Context, in *rpc.ParameterIdentifier) (*rpc.StringResponse, error) {
	return &rpc.StringResponse{Status: ok(), Value: "0"}, nil
}

func (p *param) SetParameterValue(_ context.Context, in *rpc.ParameterValue) (*rpc.CommandResponse, error) {
	p.f.SetParam(in.Parameter.ProcessorId, in.Parameter.ParameterId, in.Value)
	return &rpc.CommandResponse{Status: ok()}, nil
}

type midi struct {
	rpc.UnimplementedMidiControllerServer
	f *Fake
}

func (m *midi) GetInputPorts(context.Context, *rpc.GenericVoidValue) (*rpc.GenericIntValue, error) {
	return &rpc.GenericIntValue{Value: 1}, nil
}

func (m *midi) GetAllKbdInputConnections(context.Context, *rpc.GenericVoidValue) (*rpc.MidiKbdConnectionList, error) {
	return &rpc.MidiKbdConnectionList{}, nil
}
