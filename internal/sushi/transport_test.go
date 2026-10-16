package sushi_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/elk-audio/sushi-webui/internal/sushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/fakesushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
)

type harness struct {
	name  string
	setup func(t *testing.T, f *fakesushi.Fake) sushi.Dialer
}

var harnesses = []harness{
	{"grpc", func(t *testing.T, f *fakesushi.Fake) sushi.Dialer { return fakesushi.ServeGRPC(t, f) }},
	{"zmq", func(t *testing.T, f *fakesushi.Fake) sushi.Dialer {
		return fakesushi.StartZMQ(t, f).ZMQDialer(zmqconn.Config{})
	}},
}

// eachTransport runs a test body against the same fake Sushi over every transport.
func eachTransport(t *testing.T, body func(t *testing.T, f *fakesushi.Fake, c *sushi.Client)) {
	for _, h := range harnesses {
		t.Run(h.name, func(t *testing.T) {
			f := fakesushi.New()
			c, err := h.setup(t, f).Dial()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = c.Close() })
			body(t, f, c)
		})
	}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestUnary(t *testing.T) {
	eachTransport(t, func(t *testing.T, f *fakesushi.Fake, c *sushi.Client) {
		v, err := c.System.GetSushiVersion(ctx(t), sushi.Void)
		if err != nil || v.Value != "1.4.0-fake" {
			t.Fatalf("version = %v, %v", v, err)
		}
		if _, err := c.Transport.SetTempo(ctx(t), &rpc.GenericFloatValue{Value: 98.5}); err != nil {
			t.Fatal(err)
		}
		tempo, err := c.Transport.GetTempo(ctx(t), sushi.Void)
		if err != nil || tempo.Value != 98.5 {
			t.Fatalf("tempo = %v, %v", tempo, err)
		}
		if _, err := c.TrackID("nope"); !sushi.IsNotFound(err) {
			t.Fatalf("missing track: want NOT_FOUND, got %v", err)
		}
	})
}

func TestAsyncCommandCompletes(t *testing.T) {
	eachTransport(t, func(t *testing.T, f *fakesushi.Fake, c *sushi.Client) {
		lctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() { _ = c.RunAsyncListener(lctx) }()
		// Cmd blocks until the async notification arrives, so it only returns if streaming works.
		resp, err := c.Graph.CreateTrack(ctx(t), &rpc.CreateTrackRequest{Name: "Audio 1", Channels: 2})
		if err := c.Cmd("CreateTrack", resp, err); err != nil {
			t.Fatal(err)
		}
		id, err := sushi.WaitFor(time.Second, func() (int32, error) { return c.TrackID("Audio 1") })
		if err != nil || id == 0 {
			t.Fatalf("track id = %d, %v", id, err)
		}
		all, err := c.Graph.GetAllTracks(ctx(t), sushi.Void)
		if err != nil || len(all.Tracks) != 1 || all.Tracks[0].Channels != 2 {
			t.Fatalf("tracks = %v, %v", all, err)
		}
	})
}

func TestNotificationStreams(t *testing.T) {
	eachTransport(t, func(t *testing.T, f *fakesushi.Fake, c *sushi.Client) {
		sctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		base := f.Sinks()
		params, err := c.Notify.SubscribeToParameterUpdates(sctx, &rpc.ParameterNotificationBlocklist{})
		if err != nil {
			t.Fatal(err)
		}
		tr, err := c.Notify.SubscribeToTransportChanges(sctx, sushi.Void)
		if err != nil {
			t.Fatal(err)
		}
		tracks, err := c.Notify.SubscribeToTrackChanges(sctx, sushi.Void)
		if err != nil {
			t.Fatal(err)
		}

		// gRPC registers server-side asynchronously after the call returns; wait for all three.
		for i := 0; f.Sinks() < base+3 && i < 200; i++ {
			if f.Sinks() == base+3 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		f.SetParam(7, 3, 0.25)
		u, err := params.Recv()
		if err != nil || u.Parameter.ProcessorId != 7 || u.Parameter.ParameterId != 3 || u.NormalizedValue != 0.25 || u.FormattedValue != "v" {
			t.Fatalf("param update = %v, %v", u, err)
		}
		// Order within a stream is preserved.
		for i := 0; i < 100; i++ {
			f.SetParam(1, 1, float32(i))
		}
		for i := 0; i < 100; i++ {
			u, err := params.Recv()
			if err != nil || u.NormalizedValue != float32(i) {
				t.Fatalf("update %d = %v, %v", i, u, err)
			}
		}

		if _, err := c.Transport.SetTempo(ctx(t), &rpc.GenericFloatValue{Value: 140}); err != nil {
			t.Fatal(err)
		}
		tu, err := tr.Recv()
		if err != nil || tu.GetTempo() != 140 {
			t.Fatalf("transport update = %v, %v", tu, err)
		}

		// Streams only see their own message type.
		f.Emit(&rpc.TrackUpdate{Action: rpc.TrackUpdate_TRACK_ADDED, Track: &rpc.TrackIdentifier{Id: 5}})
		tku, err := tracks.Recv()
		if err != nil || tku.Track.Id != 5 || tku.Action != rpc.TrackUpdate_TRACK_ADDED {
			t.Fatalf("track update = %v, %v", tku, err)
		}
	})
}

func TestStreamEndsOnCancelAndClose(t *testing.T) {
	eachTransport(t, func(t *testing.T, f *fakesushi.Fake, c *sushi.Client) {
		cctx, cancel := context.WithCancel(context.Background())
		s, err := c.Notify.SubscribeToTrackChanges(cctx, sushi.Void)
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		if _, err := s.Recv(); err == nil {
			t.Fatal("Recv after cancel must fail")
		}

		s2, err := c.Notify.SubscribeToTrackChanges(context.Background(), sushi.Void)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { _, err := s2.Recv(); done <- err }()
		time.Sleep(20 * time.Millisecond)
		_ = c.Close()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("Recv after Close must fail")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("stream did not end after Close")
		}
	})
}

// The engine fires parameter writes from many goroutines; both transports must handle that.
func TestConcurrentCalls(t *testing.T) {
	eachTransport(t, func(t *testing.T, f *fakesushi.Fake, c *sushi.Client) {
		var wg sync.WaitGroup
		for i := int32(0); i < 40; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := int32(0); j < 10; j++ {
					_, err := c.Param.SetParameterValue(ctx(t), &rpc.ParameterValue{
						Parameter: &rpc.ParameterIdentifier{ProcessorId: i, ParameterId: j}, Value: float32(i*100 + j)})
					if err != nil {
						t.Error(err)
						return
					}
				}
			}()
		}
		wg.Wait()
		for i := int32(0); i < 40; i++ {
			r, err := c.Param.GetParameterValue(ctx(t), &rpc.ParameterIdentifier{ProcessorId: i, ParameterId: 9})
			if err != nil || r.Value != float32(i*100+9) {
				t.Fatalf("param %d = %v, %v", i, r, err)
			}
		}
	})
}
