package sushi_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elk-audio/sushi-webui/internal/sushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/fakesushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func zmqClient(t *testing.T, cfg zmqconn.Config) (*fakesushi.Fake, *fakesushi.ZMQServer, *sushi.Client) {
	t.Helper()
	f := fakesushi.New()
	z := fakesushi.StartZMQ(t, f)
	c, err := z.ZMQDialer(cfg).Dial()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return f, z, c
}

// Sushi's REP loop dies on a malformed request and crashes on service 9. After a full workout the
// fake (which models both) must show neither happened.
func TestZMQNeverSendsWhatBreaksSushi(t *testing.T) {
	f, z, c := zmqClient(t, zmqconn.Config{})
	go func() { _ = c.RunAsyncListener(context.Background()) }()
	if _, err := c.Notify.SubscribeToParameterUpdates(context.Background(), &rpc.ParameterNotificationBlocklist{}); err != nil {
		t.Fatal(err)
	}
	resp, err := c.Graph.CreateTrack(ctx(t), &rpc.CreateTrackRequest{Name: "x", Channels: 2})
	if err := c.Cmd("CreateTrack", resp, err); err != nil {
		t.Fatal(err)
	}
	f.SetParam(1, 1, 1)
	// Void requests are an empty second frame, not a missing one.
	if _, err := c.System.GetSushiVersion(ctx(t), sushi.Void); err != nil {
		t.Fatal(err)
	}
	if z.Wedged() || z.CvGateCalled() {
		t.Fatalf("client broke the fake Sushi: wedged=%v cvgate=%v", z.Wedged(), z.CvGateCalled())
	}
	if _, err := c.System.GetSushiVersion(ctx(t), sushi.Void); err != nil {
		t.Fatal(err)
	}
}

func TestZMQUsesSushiWireIDs(t *testing.T) {
	_, z, c := zmqClient(t, zmqconn.Config{KeepAlive: -1})
	if _, err := c.Graph.GetTrackId(ctx(t), &rpc.GenericStringValue{Value: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Transport.SetTempo(ctx(t), &rpc.GenericFloatValue{Value: 1}); err != nil {
		t.Fatal(err)
	}
	got := z.Requests()
	want := [][2]int32{{4, 2}, {1, 5}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("requests = %v, want %v", got, want)
	}
}

func TestZMQSubscribesOncePerNotification(t *testing.T) {
	_, z, c := zmqClient(t, zmqconn.Config{KeepAlive: -1})
	for i := 0; i < 3; i++ {
		s, err := c.Notify.SubscribeToTrackChanges(context.Background(), sushi.Void)
		if err != nil {
			t.Fatal(err)
		}
		_ = s
	}
	n := 0
	for _, r := range z.Requests() {
		if r == [2]int32{12, 2} {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("SubscribeToTrackChanges sent %d times, want 1", n)
	}
}

func TestZMQTimeoutRecovers(t *testing.T) {
	_, z, c := zmqClient(t, zmqconn.Config{KeepAlive: -1})
	release := z.Stall()
	short, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := c.System.GetSushiVersion(short, sushi.Void)
	if status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("stalled call: want DeadlineExceeded, got %v", err)
	}
	release()
	// The abandoned request is still answered by the server; the client must have moved to a fresh
	// socket so that stale reply cannot be mistaken for this call's reply.
	r, err := c.Graph.GetTrackId(ctx(t), &rpc.GenericStringValue{Value: "nope"})
	if err != nil || r.Status.Status != rpc.CommandStatus_NOT_FOUND {
		t.Fatalf("call after timeout = %v, %v", r, err)
	}
}

func TestZMQDialFailsFastWithoutSushi(t *testing.T) {
	dir, _ := os.MkdirTemp("", "z")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	start := time.Now()
	_, err := sushi.ZMQDialer(zmqconn.Config{Control: "ipc://" + filepath.Join(dir, "c"), Notify: "ipc://" + filepath.Join(dir, "p")}).Dial()
	if err == nil {
		t.Fatal("Dial must fail when Sushi is not running")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("Dial took %v", time.Since(start))
	}
}

// With Sushi gone nothing on a SUB socket reports it, so the keepalive must end the streams —
// that is what makes the engine's supervisor reconnect.
func TestZMQStreamsEndWhenSushiDies(t *testing.T) {
	f := fakesushi.New()
	z := fakesushi.StartZMQ(t, f)
	c, err := z.ZMQDialer(zmqconn.Config{KeepAlive: 100 * time.Millisecond, CallTimeout: 300 * time.Millisecond}).Dial()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	s, err := c.Notify.SubscribeToTrackChanges(context.Background(), sushi.Void)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := s.Recv(); done <- err }()
	z.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not end after Sushi died")
	}
	if _, err := c.System.GetSushiVersion(ctx(t), sushi.Void); err == nil {
		t.Fatal("calls on a dead connection must fail")
	}
}
