// Package fakesushi is an in-memory stand-in for a Sushi instance, served over either transport
// (gRPC through bufconn, or Sushi's ZeroMQ protocol over ipc sockets). It exists so that the same
// tests can drive the real client code against both transports without a running Sushi.
package fakesushi

import (
	"context"
	"sync"
	"time"

	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func ok() *rpc.CommandStatus { return &rpc.CommandStatus{Status: rpc.CommandStatus_SUCCESS} }

// Fake holds the state of the pretend Sushi.
type Fake struct {
	mu       sync.Mutex
	nextID   int32
	nextReq  int32
	tracks   []*trackRec
	params   map[[2]int32]float32
	tempo    float32
	sinks    map[*sink]struct{}
	calls    map[string]int
	AsyncLag time.Duration // delay before an async command completes
}

type sink struct{ fn func(proto.Message) }

// New returns a Fake with the defaults a real Sushi would report.
func New() *Fake {
	return &Fake{nextID: 1, params: map[[2]int32]float32{}, tempo: 120, sinks: map[*sink]struct{}{},
		calls: map[string]int{}, AsyncLag: 5 * time.Millisecond}
}

// Calls returns how often each RPC (by method name) reached the fake.
func (f *Fake) Calls(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *Fake) count(m string) {
	f.mu.Lock()
	f.calls[m]++
	f.mu.Unlock()
}

// Sinks is the number of live notification subscribers (gRPC streams plus the ZeroMQ publisher).
// Tests use it to wait until server-side subscriptions are in place before emitting.
func (f *Fake) Sinks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sinks)
}

// Emit publishes a notification to every live subscriber (on either transport).
func (f *Fake) Emit(m proto.Message) {
	f.mu.Lock()
	var fns []func(proto.Message)
	for s := range f.sinks {
		fns = append(fns, s.fn)
	}
	f.mu.Unlock()
	for _, fn := range fns {
		fn(m)
	}
}

func (f *Fake) addSink(fn func(proto.Message)) func() {
	s := &sink{fn}
	f.mu.Lock()
	f.sinks[s] = struct{}{}
	f.mu.Unlock()
	return func() { f.mu.Lock(); delete(f.sinks, s); f.mu.Unlock() }
}

// SetParam stores a parameter value and notifies subscribers, like a plugin changing its own value.
func (f *Fake) SetParam(proc, param int32, v float32) {
	f.mu.Lock()
	f.params[[2]int32{proc, param}] = v
	f.mu.Unlock()
	f.Emit(&rpc.ParameterUpdate{Parameter: &rpc.ParameterIdentifier{ProcessorId: proc, ParameterId: param},
		NormalizedValue: v, DomainValue: v, FormattedValue: "v"})
}

// Register adds every service the fake implements to a gRPC-style registrar.
func (f *Fake) Register(r grpc.ServiceRegistrar) {
	rpc.RegisterSystemControllerServer(r, &system{f: f})
	rpc.RegisterTransportControllerServer(r, &transport{f: f})
	rpc.RegisterTimingControllerServer(r, &timing{f: f})
	rpc.RegisterAudioGraphControllerServer(r, &graph{f: f})
	rpc.RegisterParameterControllerServer(r, &param{f: f})
	rpc.RegisterMidiControllerServer(r, &midi{f: f})
	rpc.RegisterNotificationControllerServer(r, &notify{f: f})
}

type system struct {
	rpc.UnimplementedSystemControllerServer
	f *Fake
}

func (s *system) GetSushiVersion(context.Context, *rpc.GenericVoidValue) (*rpc.GenericStringValue, error) {
	s.f.count("GetSushiVersion")
	return &rpc.GenericStringValue{Value: "1.4.0-fake"}, nil
}
func (s *system) GetSushiApiVersion(context.Context, *rpc.GenericVoidValue) (*rpc.GenericStringValue, error) {
	return &rpc.GenericStringValue{Value: "1.0.0"}, nil
}
func (s *system) GetInputAudioChannelCount(context.Context, *rpc.GenericVoidValue) (*rpc.GenericIntValue, error) {
	return &rpc.GenericIntValue{Value: 2}, nil
}
func (s *system) GetOutputAudioChannelCount(context.Context, *rpc.GenericVoidValue) (*rpc.GenericIntValue, error) {
	return &rpc.GenericIntValue{Value: 4}, nil
}

type transport struct {
	rpc.UnimplementedTransportControllerServer
	f *Fake
}

func (t *transport) GetSamplerate(context.Context, *rpc.GenericVoidValue) (*rpc.GenericFloatValue, error) {
	return &rpc.GenericFloatValue{Value: 48000}, nil
}
func (t *transport) GetTempo(context.Context, *rpc.GenericVoidValue) (*rpc.GenericFloatValue, error) {
	t.f.mu.Lock()
	defer t.f.mu.Unlock()
	return &rpc.GenericFloatValue{Value: t.f.tempo}, nil
}
func (t *transport) SetTempo(_ context.Context, in *rpc.GenericFloatValue) (*rpc.CommandResponse, error) {
	t.f.mu.Lock()
	t.f.tempo = in.Value
	t.f.mu.Unlock()
	t.f.Emit(&rpc.TransportUpdate{Transport: &rpc.TransportUpdate_Tempo{Tempo: in.Value}})
	return &rpc.CommandResponse{Status: ok()}, nil
}

type timing struct {
	rpc.UnimplementedTimingControllerServer
	f *Fake
}

func (t *timing) SetTimingsEnabled(context.Context, *rpc.GenericBoolValue) (*rpc.CommandResponse, error) {
	return &rpc.CommandResponse{Status: ok()}, nil
}

// notify implements the gRPC subscriptions; the ZeroMQ server handles the Subscribe* calls itself.
type notify struct {
	rpc.UnimplementedNotificationControllerServer
	f *Fake
}

func stream[T proto.Message](f *Fake, ctx context.Context, send func(T) error) error {
	ch := make(chan T, 4096)
	rm := f.addSink(func(m proto.Message) {
		if t, isT := m.(T); isT {
			select {
			case ch <- t:
			default:
			}
		}
	})
	defer rm()
	for {
		select {
		case m := <-ch:
			if err := send(m); err != nil {
				return err
			}
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		}
	}
}

func (n *notify) SubscribeToTransportChanges(_ *rpc.GenericVoidValue, s grpc.ServerStreamingServer[rpc.TransportUpdate]) error {
	return stream(n.f, s.Context(), s.Send)
}
func (n *notify) SubscribeToEngineCpuTimingUpdates(_ *rpc.GenericVoidValue, s grpc.ServerStreamingServer[rpc.CpuTimings]) error {
	return stream(n.f, s.Context(), s.Send)
}
func (n *notify) SubscribeToTrackChanges(_ *rpc.GenericVoidValue, s grpc.ServerStreamingServer[rpc.TrackUpdate]) error {
	return stream(n.f, s.Context(), s.Send)
}
func (n *notify) SubscribeToProcessorChanges(_ *rpc.GenericVoidValue, s grpc.ServerStreamingServer[rpc.ProcessorUpdate]) error {
	return stream(n.f, s.Context(), s.Send)
}
func (n *notify) SubscribeToParameterUpdates(_ *rpc.ParameterNotificationBlocklist, s grpc.ServerStreamingServer[rpc.ParameterUpdate]) error {
	return stream(n.f, s.Context(), s.Send)
}
func (n *notify) SubscribeToAsyncCommandUpdates(_ *rpc.GenericVoidValue, s grpc.ServerStreamingServer[rpc.AsyncCommandResponse]) error {
	return stream(n.f, s.Context(), s.Send)
}

// TransportTempo changes the tempo as if another client had, notifying subscribers.
func (f *Fake) TransportTempo(v float32) (float32, error) {
	t := &transport{f: f}
	_, err := t.SetTempo(context.Background(), &rpc.GenericFloatValue{Value: v})
	return v, err
}
