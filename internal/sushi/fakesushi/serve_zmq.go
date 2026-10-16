package fakesushi

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/elk-audio/sushi-webui/internal/sushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"github.com/go-zeromq/zmq4"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ZMQServer speaks Sushi's ZeroMQ protocol, decoding requests the way zmq_server.cpp does, and
// reproduces its failure modes so tests can assert the client never triggers them.
type ZMQServer struct {
	Control, Notify string

	f      *Fake
	ctx    context.Context
	cancel context.CancelFunc
	rep    zmq4.Socket
	pub    zmq4.Socket
	pubMu  sync.Mutex
	rm     func()

	mu         sync.Mutex
	subscribed map[int32]bool
	wedged     bool
	cvGate     bool
	requests   [][2]int32
	hold       chan struct{} // when non-nil, replies wait for it (simulates a stalled Sushi)
}

// Wedged reports whether a malformed request arrived. A real Sushi stops answering after one.
func (z *ZMQServer) Wedged() bool { z.mu.Lock(); defer z.mu.Unlock(); return z.wedged }

// CvGateCalled reports whether service 9 was requested (a real Sushi would crash).
func (z *ZMQServer) CvGateCalled() bool { z.mu.Lock(); defer z.mu.Unlock(); return z.cvGate }

// Requests returns the {service, endpoint} pairs received so far.
func (z *ZMQServer) Requests() [][2]int32 {
	z.mu.Lock()
	defer z.mu.Unlock()
	return append([][2]int32(nil), z.requests...)
}

// Stall makes the server hold replies until the returned function is called.
func (z *ZMQServer) Stall() (release func()) {
	ch := make(chan struct{})
	z.mu.Lock()
	z.hold = ch
	z.mu.Unlock()
	return func() { z.mu.Lock(); z.hold = nil; z.mu.Unlock(); close(ch) }
}

// Close stops the server and removes its sockets.
func (z *ZMQServer) Close() {
	z.rm()
	z.cancel()
	_ = z.rep.Close()
	_ = z.pub.Close()
}

// StartZMQ serves the fake on fresh ipc sockets. The paths are kept short because unix socket
// paths are limited to ~100 bytes on macOS.
func StartZMQ(tb testing.TB, f *Fake) *ZMQServer {
	tb.Helper()
	dir, err := os.MkdirTemp("", "z")
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })
	return StartZMQAt(tb, f, "ipc://"+filepath.Join(dir, "c"), "ipc://"+filepath.Join(dir, "p"))
}

// StartZMQAt is StartZMQ on explicit endpoints (to restart a server where a client expects it).
func StartZMQAt(tb testing.TB, f *Fake, control, notify string) *ZMQServer {
	tb.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	z := &ZMQServer{Control: control, Notify: notify, f: f, ctx: ctx, cancel: cancel, subscribed: map[int32]bool{}}
	z.rep = zmq4.NewRouter(ctx)
	z.pub = zmq4.NewPub(ctx)
	if err := z.rep.Listen(control); err != nil {
		tb.Fatal(err)
	}
	if err := z.pub.Listen(notify); err != nil {
		tb.Fatal(err)
	}
	reg := &registry{svcs: map[string]*serviceImpl{}}
	f.Register(reg)
	z.rm = f.addSink(z.publish)
	go z.serve(reg)
	tb.Cleanup(z.Close)
	return z
}

// ZMQDialer returns a Dialer for the server, using the real zmqconn client.
func (z *ZMQServer) ZMQDialer(cfg zmqconn.Config) sushi.Dialer {
	cfg.Control, cfg.Notify = z.Control, z.Notify
	return sushi.ZMQDialer(cfg)
}

func (z *ZMQServer) publish(m proto.Message) {
	code, ok := codeOf(m)
	if !ok {
		return
	}
	z.mu.Lock()
	on := z.subscribed[code]
	z.mu.Unlock()
	if !on {
		return // Sushi only publishes what was subscribed to
	}
	b, err := proto.Marshal(m)
	if err != nil {
		return
	}
	var c [4]byte
	binary.NativeEndian.PutUint32(c[:], uint32(code))
	z.pubMu.Lock()
	defer z.pubMu.Unlock()
	_ = z.pub.SendMulti(zmq4.NewMsgFrom(c[:], b))
}

func codeOf(m proto.Message) (int32, bool) {
	name := m.ProtoReflect().Descriptor().FullName()
	msgs := rpc.File_sushi_rpc_proto.Messages()
	for i := 0; i < msgs.Len(); i++ {
		if msgs.Get(i).FullName() == name {
			return int32(i), true
		}
	}
	return 0, false
}

type serviceImpl struct {
	desc *grpc.ServiceDesc
	impl any
}

// registry collects the fake's services as a grpc.ServiceRegistrar.
type registry struct{ svcs map[string]*serviceImpl }

func (r *registry) RegisterService(d *grpc.ServiceDesc, impl any) {
	r.svcs[d.ServiceName] = &serviceImpl{d, impl}
}

func (z *ZMQServer) serve(reg *registry) {
	files := rpc.File_sushi_rpc_proto.Services()
	for z.ctx.Err() == nil {
		// A ROUTER shows the REQ envelope explicitly: [identity][empty delimiter][request frames...].
		// (zmq4's own REP takes the *last* empty frame as the delimiter, which breaks requests that
		// end in an empty payload; libzmq's REP, which Sushi uses, takes the first.)
		raw, err := z.rep.Recv()
		if err != nil {
			return
		}
		if len(raw.Frames) < 2 || len(raw.Frames[1]) != 0 {
			continue
		}
		env := raw.Frames[:2]
		m := zmq4.NewMsgFrom(raw.Frames[2:]...)
		// A real Sushi never answers again after a bad frame (REP stays in "received" state).
		if len(m.Frames) != 2 || len(m.Frames[0]) != 8 {
			z.mu.Lock()
			z.wedged = true
			z.mu.Unlock()
			<-z.ctx.Done()
			return
		}
		svc := int32(binary.NativeEndian.Uint32(m.Frames[0][0:]))
		ep := int32(binary.NativeEndian.Uint32(m.Frames[0][4:]))
		z.mu.Lock()
		z.requests = append(z.requests, [2]int32{svc, ep})
		hold := z.hold
		if svc == 9 {
			z.cvGate = true
			z.wedged = true
			z.mu.Unlock()
			<-z.ctx.Done()
			return
		}
		z.mu.Unlock()
		if hold != nil {
			select {
			case <-hold:
			case <-z.ctx.Done():
				return
			}
		}
		if int(svc) >= files.Len() || svc < 0 || ep < 0 || int(ep) >= files.Get(int(svc)).Methods().Len() {
			z.reply(env, nil) // Sushi answers unknown commands with an empty frame
			continue
		}
		sd := files.Get(int(svc))
		md := sd.Methods().Get(int(ep))
		var out []byte
		if sd.FullName() == "sushi_rpc.NotificationController" {
			// Handled by the server itself: flip the global subscription flag.
			code, _ := msgCodeOfName(string(md.Output().FullName()))
			z.mu.Lock()
			z.subscribed[code] = true
			z.mu.Unlock()
		} else {
			si := reg.svcs[string(sd.FullName())]
			var resp any
			var herr error
			if si != nil {
				for _, h := range si.desc.Methods {
					if h.MethodName == string(md.Name()) {
						resp, herr = h.Handler(si.impl, z.ctx, func(v any) error { return proto.Unmarshal(m.Frames[1], v.(proto.Message)) }, nil)
					}
				}
			}
			if pm, ok := resp.(proto.Message); ok && herr == nil {
				out, _ = proto.Marshal(pm)
			} else {
				out = unsupported(md.Output())
			}
		}
		z.reply(env, out)
	}
}

// unsupported builds a reply carrying UNSUPPORTED_OPERATION (when the response type has a status
// field), so a method the fake lacks fails loudly rather than looking like a success.
func unsupported(d protoreflect.MessageDescriptor) []byte {
	m := dynamicpb.NewMessage(d)
	if f := d.Fields().ByName("status"); f != nil && f.Message() != nil {
		st := dynamicpb.NewMessage(f.Message())
		if sf := f.Message().Fields().ByName("status"); sf != nil {
			st.Set(sf, protoreflect.ValueOfEnum(protoreflect.EnumNumber(rpc.CommandStatus_UNSUPPORTED_OPERATION)))
			m.Set(f, protoreflect.ValueOfMessage(st))
		}
	}
	b, _ := proto.Marshal(m)
	return b
}

func (z *ZMQServer) reply(env [][]byte, body []byte) {
	_ = z.rep.SendMulti(zmq4.NewMsgFrom(env[0], env[1], body))
}

func msgCodeOfName(full string) (int32, bool) {
	msgs := rpc.File_sushi_rpc_proto.Messages()
	for i := 0; i < msgs.Len(); i++ {
		if string(msgs.Get(i).FullName()) == full {
			return int32(i), true
		}
	}
	return 0, false
}
