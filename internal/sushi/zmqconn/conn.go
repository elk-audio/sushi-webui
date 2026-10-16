// Package zmqconn talks to Sushi's ZeroMQ control interface and presents it as a
// grpc.ClientConnInterface, so the generated gRPC clients (and everything built on them) work
// unchanged over either transport.
//
// Wire protocol (Sushi 1.4, rpc_interface/src/zmq_server.cpp):
//   - Control: REQ/REP. Request is two frames: an 8-byte header {int32 service, int32 endpoint}
//     in native byte order, then the protobuf request. The reply is one frame with the protobuf
//     response. Strictly one request at a time.
//   - Notifications: PUB/SUB. Frames are {int32 message code, protobuf payload}. Subscriptions are
//     switched on globally with the NotificationController.SubscribeTo* control calls.
//
// service and endpoint are the declaration order of the service and of the rpc in sushi_rpc.proto,
// and the message code is the index of the message among the proto's top-level messages. All are
// derived from the compiled descriptors rather than hard-coded.
//
// Sushi's REP loop wedges permanently on a malformed request and crashes on the (unimplemented)
// CvGateController, so this package only ever sends well-formed requests and refuses that service.
package zmqconn

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"sync"
	"time"

	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"github.com/go-zeromq/zmq4"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Default endpoints, hard-coded in Sushi.
const (
	DefaultControl = "ipc:///tmp/sushi_ipc.sock"
	DefaultNotify  = "ipc:///tmp/sushi_pub.sock"
)

// Config selects the endpoints and timing of a connection. Zero values get defaults.
type Config struct {
	Control string
	Notify  string
	// CallTimeout bounds a control call whose context has no deadline.
	CallTimeout time.Duration
	// KeepAlive is the interval of the liveness probe. The SUB socket cannot tell that Sushi went
	// away, so streams are ended when the probe fails twice in a row. Negative disables it.
	KeepAlive time.Duration
	// StreamBuffer is the per-stream notification queue length; overflow drops notifications.
	StreamBuffer int
}

func (c *Config) defaults() {
	if c.Control == "" {
		c.Control = DefaultControl
	}
	if c.Notify == "" {
		c.Notify = DefaultNotify
	}
	if c.CallTimeout == 0 {
		c.CallTimeout = 5 * time.Second
	}
	if c.KeepAlive == 0 {
		c.KeepAlive = 2 * time.Second
	}
	if c.StreamBuffer == 0 {
		c.StreamBuffer = 8192
	}
}

type endpoint struct {
	service, endpoint int32
	method            protoreflect.MethodDescriptor
}

var (
	endpoints  = map[string]endpoint{} // "/sushi_rpc.Svc/Method" → wire ids
	msgCodes   = map[protoreflect.FullName]int32{}
	blockedSvc = protoreflect.FullName("sushi_rpc.CvGateController")
)

func init() {
	f := rpc.File_sushi_rpc_proto
	for i := 0; i < f.Services().Len(); i++ {
		s := f.Services().Get(i)
		for j := 0; j < s.Methods().Len(); j++ {
			m := s.Methods().Get(j)
			endpoints[fmt.Sprintf("/%s/%s", s.FullName(), m.Name())] = endpoint{int32(i), int32(j), m}
		}
	}
	for i := 0; i < f.Messages().Len(); i++ {
		msgCodes[f.Messages().Get(i).FullName()] = int32(i)
	}
}

func protoName(n string) protoreflect.FullName { return protoreflect.FullName(n) }

// Conn is a ZeroMQ connection to one Sushi instance.
type Conn struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc

	reqMu sync.Mutex // serialises control calls (REQ/REP is lock-step)

	subMu      sync.Mutex // guards subscribed, held across the subscribe call
	subscribed map[int32]bool

	sub zmq4.Socket

	mu      sync.Mutex
	req     zmq4.Socket                    // current REQ socket, nil after a failure
	streams map[int32]map[*stream]struct{} // message code → live streams
	err     error                          // set once the connection is dead
	dropped int
}

var _ grpc.ClientConnInterface = (*Conn)(nil)

// Dial connects to Sushi. Unlike gRPC the sockets are real ipc connections, so it fails fast when
// Sushi is not running.
func Dial(cfg Config) (*Conn, error) {
	cfg.defaults()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Conn{cfg: cfg, ctx: ctx, cancel: cancel, subscribed: map[int32]bool{}, streams: map[int32]map[*stream]struct{}{}}
	if err := c.resetReq(); err != nil {
		cancel()
		return nil, err
	}
	sub := zmq4.NewSub(ctx, zmq4.WithDialerMaxRetries(0))
	if err := sub.SetOption(zmq4.OptionSubscribe, ""); err != nil {
		cancel()
		_ = c.getReq().Close()
		return nil, err
	}
	if err := sub.Dial(cfg.Notify); err != nil {
		cancel()
		_ = c.getReq().Close()
		return nil, err
	}
	c.sub = sub
	go c.readNotifications()
	if cfg.KeepAlive > 0 {
		go c.keepAlive()
	}
	return c, nil
}

// Close shuts both sockets and ends all streams.
func (c *Conn) Close() error {
	c.fail(status.Error(codes.Canceled, "connection closed"))
	return nil
}

// fail marks the connection dead, ends the streams and closes the sockets. Only the first call counts.
func (c *Conn) fail(err error) {
	c.mu.Lock()
	if c.err != nil {
		c.mu.Unlock()
		return
	}
	c.err = err
	all := c.streams
	c.streams = map[int32]map[*stream]struct{}{}
	c.mu.Unlock()
	for _, set := range all {
		for s := range set {
			s.finish(err)
		}
	}
	c.cancel()
	if c.sub != nil {
		_ = c.sub.Close()
	}
	// reqMu may be held by a call blocked in Recv; closing outside the lock unblocks it.
	if r := c.getReq(); r != nil {
		_ = r.Close()
	}
}

func (c *Conn) getReq() zmq4.Socket {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.req
}

func (c *Conn) setReq(r zmq4.Socket) {
	c.mu.Lock()
	c.req = r
	c.mu.Unlock()
}

func (c *Conn) deadErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// ---- control channel ----

// resetReq (re)creates the REQ socket. Callers hold reqMu, except Dial.
func (c *Conn) resetReq() error {
	if old := c.getReq(); old != nil {
		_ = old.Close()
	}
	r := zmq4.NewReq(c.ctx, zmq4.WithDialerMaxRetries(0))
	if err := r.Dial(c.cfg.Control); err != nil {
		c.setReq(nil)
		_ = r.Close()
		return err
	}
	c.setReq(r)
	return nil
}

// call performs one lock-step request/reply. A timeout or socket error discards the REQ socket
// (a REQ cannot be reused after a missed reply) and the next call opens a fresh one.
func (c *Conn) call(ctx context.Context, ep endpoint, payload []byte) ([]byte, error) {
	c.reqMu.Lock()
	defer c.reqMu.Unlock()
	if err := c.deadErr(); err != nil {
		return nil, err
	}
	if c.getReq() == nil {
		if err := c.resetReq(); err != nil {
			return nil, status.Error(codes.Unavailable, err.Error())
		}
	}
	timeout := c.cfg.CallTimeout
	if dl, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(dl))
	}
	if timeout <= 0 {
		return nil, status.FromContextError(context.DeadlineExceeded).Err()
	}

	req := c.getReq()
	type result struct {
		frames [][]byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		var hdr [8]byte
		binary.NativeEndian.PutUint32(hdr[0:], uint32(ep.service))
		binary.NativeEndian.PutUint32(hdr[4:], uint32(ep.endpoint))
		if err := req.SendMulti(zmq4.NewMsgFrom(hdr[:], payload)); err != nil {
			done <- result{err: err}
			return
		}
		m, err := req.Recv()
		done <- result{m.Frames, err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-done:
		if r.err != nil {
			c.dropReq()
			return nil, status.Error(codes.Unavailable, r.err.Error())
		}
		if len(r.frames) != 1 {
			c.dropReq()
			return nil, status.Errorf(codes.Internal, "zmq reply has %d frames, want 1", len(r.frames))
		}
		return r.frames[0], nil
	case <-timer.C:
		c.dropReq()
		return nil, status.Error(codes.DeadlineExceeded, "sushi did not reply")
	case <-ctx.Done():
		c.dropReq()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

func (c *Conn) dropReq() {
	if r := c.getReq(); r != nil {
		_ = r.Close()
	}
	c.setReq(nil)
}

// Invoke implements grpc.ClientConnInterface for unary methods.
func (c *Conn) Invoke(ctx context.Context, method string, args, reply any, _ ...grpc.CallOption) error {
	ep, err := lookup(method)
	if err != nil {
		return err
	}
	in, ok := args.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "%s: request is not a proto message", method)
	}
	out, ok := reply.(proto.Message)
	if !ok {
		return status.Errorf(codes.Internal, "%s: reply is not a proto message", method)
	}
	payload, err := proto.Marshal(in)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	b, err := c.call(ctx, ep, payload)
	if err != nil {
		return err
	}
	if err := proto.Unmarshal(b, out); err != nil {
		return status.Errorf(codes.Internal, "%s: bad reply: %v", method, err)
	}
	return nil
}

func lookup(method string) (endpoint, error) {
	ep, ok := endpoints[method]
	if !ok {
		return endpoint{}, status.Errorf(codes.Unimplemented, "unknown method %s", method)
	}
	if ep.method.Parent().FullName() == blockedSvc {
		return endpoint{}, status.Errorf(codes.Unimplemented, "%s is not available over ZeroMQ (Sushi crashes)", method)
	}
	return ep, nil
}

func (c *Conn) keepAlive() {
	ep := endpoints["/sushi_rpc.SystemController/GetSushiVersion"]
	t := time.NewTicker(c.cfg.KeepAlive)
	defer t.Stop()
	misses := 0
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-t.C:
		}
		ctx, cancel := context.WithTimeout(c.ctx, max(c.cfg.KeepAlive, time.Second))
		_, err := c.call(ctx, ep, nil)
		cancel()
		if c.ctx.Err() != nil {
			return
		}
		if err != nil {
			if misses++; misses >= 2 {
				slog.Warn("sushi zmq keepalive failed", "err", err)
				c.fail(status.Error(codes.Unavailable, "sushi stopped responding"))
				return
			}
			continue
		}
		misses = 0
	}
}

// ---- notifications ----

type stream struct {
	c    *Conn
	ctx  context.Context
	code int32
	ep   endpoint
	ch   chan []byte
	once sync.Once
	err  error // valid once done is closed
	done chan struct{}
}

var _ grpc.ClientStream = (*stream)(nil)

func (s *stream) finish(err error) {
	s.once.Do(func() { s.err = err; close(s.done) })
}

func (s *stream) Header() (metadata.MD, error) { return nil, nil }
func (s *stream) Trailer() metadata.MD         { return nil }
func (s *stream) CloseSend() error             { return nil }
func (s *stream) Context() context.Context     { return s.ctx }

// SendMsg carries the subscribe request: it switches the notification on in Sushi. The blocklist
// of the parameter/property subscriptions is ignored by Sushi over ZeroMQ.
func (s *stream) SendMsg(m any) error {
	in, ok := m.(proto.Message)
	if !ok {
		return status.Error(codes.Internal, "subscribe request is not a proto message")
	}
	payload, err := proto.Marshal(in)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	return s.c.subscribe(s.ctx, s.ep, s.code, payload)
}

func (s *stream) RecvMsg(m any) error {
	// Drain queued notifications before reporting that the stream ended.
	select {
	case b := <-s.ch:
		return proto.Unmarshal(b, m.(proto.Message))
	default:
	}
	select {
	case b := <-s.ch:
		return proto.Unmarshal(b, m.(proto.Message))
	case <-s.done:
		return s.err
	case <-s.ctx.Done():
		s.c.remove(s)
		return status.FromContextError(s.ctx.Err()).Err()
	}
}

// NewStream implements grpc.ClientConnInterface for the NotificationController subscriptions.
func (c *Conn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	ep, err := lookup(method)
	if err != nil {
		return nil, err
	}
	if !desc.ServerStreams || desc.ClientStreams || ep.method.Parent().FullName() != "sushi_rpc.NotificationController" {
		return nil, status.Errorf(codes.Unimplemented, "%s: only notification subscriptions can stream", method)
	}
	code, ok := msgCodes[ep.method.Output().FullName()]
	if !ok {
		return nil, status.Errorf(codes.Internal, "no message code for %s", ep.method.Output().FullName())
	}
	s := &stream{c: c, ctx: ctx, code: code, ep: ep, ch: make(chan []byte, c.cfg.StreamBuffer), done: make(chan struct{})}
	// Register before the subscribe call so nothing published right after it is missed.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return nil, c.err
	}
	if c.streams[code] == nil {
		c.streams[code] = map[*stream]struct{}{}
	}
	c.streams[code][s] = struct{}{}
	return s, nil
}

func (c *Conn) remove(s *stream) {
	c.mu.Lock()
	delete(c.streams[s.code], s)
	c.mu.Unlock()
}

// subscribe enables one notification type in Sushi, once per connection. The subscription is
// global and permanent on Sushi's side.
func (c *Conn) subscribe(ctx context.Context, ep endpoint, code int32, payload []byte) error {
	c.subMu.Lock()
	defer c.subMu.Unlock()
	if c.subscribed[code] {
		return nil
	}
	if _, err := c.call(ctx, ep, payload); err != nil {
		return err
	}
	c.subscribed[code] = true
	return nil
}

func (c *Conn) readNotifications() {
	for {
		m, err := c.sub.Recv()
		if err != nil {
			c.fail(status.Error(codes.Unavailable, err.Error()))
			return
		}
		if len(m.Frames) != 2 || len(m.Frames[0]) != 4 {
			slog.Debug("sushi zmq: unexpected notification framing", "frames", len(m.Frames))
			continue
		}
		code := int32(binary.NativeEndian.Uint32(m.Frames[0]))
		c.mu.Lock()
		for s := range c.streams[code] {
			select {
			case s.ch <- m.Frames[1]:
			default:
				if c.dropped++; c.dropped == 1 || c.dropped%1000 == 0 {
					slog.Warn("sushi zmq: notification queue full, dropping", "code", code, "dropped", c.dropped)
				}
			}
		}
		c.mu.Unlock()
	}
}
