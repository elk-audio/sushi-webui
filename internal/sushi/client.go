// Package sushi wraps the generated gRPC stubs with the few helpers the WebUI needs:
// status checking, waiting for asynchronous commands, and name lookups.
package sushi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Void is the empty request message.
var Void = &rpc.GenericVoidValue{}

// Client bundles the per-service gRPC clients of one Sushi connection.
type Client struct {
	Conn      *grpc.ClientConn
	System    rpc.SystemControllerClient
	Transport rpc.TransportControllerClient
	Timing    rpc.TimingControllerClient
	Graph     rpc.AudioGraphControllerClient
	Program   rpc.ProgramControllerClient
	Param     rpc.ParameterControllerClient
	Midi      rpc.MidiControllerClient
	Routing   rpc.AudioRoutingControllerClient
	Session   rpc.SessionControllerClient
	Notify    rpc.NotificationControllerClient

	asyncMu   sync.Mutex
	asyncDone map[int32]chan rpc.CommandStatus_Status
}

// Dial creates a client for the given address. The connection is lazy; use Ping to check it.
func Dial(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Client{
		Conn:      conn,
		System:    rpc.NewSystemControllerClient(conn),
		Transport: rpc.NewTransportControllerClient(conn),
		Timing:    rpc.NewTimingControllerClient(conn),
		Graph:     rpc.NewAudioGraphControllerClient(conn),
		Program:   rpc.NewProgramControllerClient(conn),
		Param:     rpc.NewParameterControllerClient(conn),
		Midi:      rpc.NewMidiControllerClient(conn),
		Routing:   rpc.NewAudioRoutingControllerClient(conn),
		Session:   rpc.NewSessionControllerClient(conn),
		Notify:    rpc.NewNotificationControllerClient(conn),
		asyncDone: map[int32]chan rpc.CommandStatus_Status{},
	}, nil
}

func (c *Client) Close() error { return c.Conn.Close() }

// Ctx returns a context with a short timeout for unary calls.
func Ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}

// StatusError is a non-success Sushi command status.
type StatusError struct {
	Op     string
	Status rpc.CommandStatus_Status
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: %s", e.Op, e.Status) }

// Check turns a CommandStatus into an error. SUCCESS and ASYNC_RESPONSE are not errors.
func Check(op string, st *rpc.CommandStatus) error {
	if st == nil {
		return nil
	}
	switch st.Status {
	case rpc.CommandStatus_SUCCESS, rpc.CommandStatus_ASYNC_RESPONSE, rpc.CommandStatus_DUMMY:
		return nil
	}
	return &StatusError{op, st.Status}
}

// Cmd checks a CommandResponse and, for asynchronous commands, waits for completion.
func (c *Client) Cmd(op string, resp *rpc.CommandResponse, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if err := Check(op, resp.Status); err != nil {
		return err
	}
	if resp.Status != nil && resp.Status.Status == rpc.CommandStatus_ASYNC_RESPONSE {
		return c.waitAsync(op, resp.Id)
	}
	return nil
}

// RunAsyncListener consumes the async command stream and wakes waiters. Returns when the stream ends.
func (c *Client) RunAsyncListener(ctx context.Context) error {
	stream, err := c.Notify.SubscribeToAsyncCommandUpdates(ctx, Void)
	if err != nil {
		return err
	}
	for {
		u, err := stream.Recv()
		if err != nil {
			return err
		}
		c.asyncMu.Lock()
		ch, ok := c.asyncDone[u.RequestId]
		if !ok {
			ch = make(chan rpc.CommandStatus_Status, 1)
			c.asyncDone[u.RequestId] = ch
		}
		c.asyncMu.Unlock()
		st := rpc.CommandStatus_SUCCESS
		if u.Status != nil {
			st = u.Status.Status
		}
		select {
		case ch <- st:
		default:
		}
	}
}

func (c *Client) waitAsync(op string, id int32) error {
	c.asyncMu.Lock()
	ch, ok := c.asyncDone[id]
	if !ok {
		ch = make(chan rpc.CommandStatus_Status, 1)
		c.asyncDone[id] = ch
	}
	c.asyncMu.Unlock()
	defer func() { c.asyncMu.Lock(); delete(c.asyncDone, id); c.asyncMu.Unlock() }()
	select {
	case st := <-ch:
		if st != rpc.CommandStatus_SUCCESS {
			return &StatusError{op, st}
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("%s: timed out waiting for Sushi", op)
	}
}

// TrackID looks a track up by name.
func (c *Client) TrackID(name string) (int32, error) {
	ctx, cancel := Ctx()
	defer cancel()
	r, err := c.Graph.GetTrackId(ctx, &rpc.GenericStringValue{Value: name})
	if err != nil {
		return 0, err
	}
	if err := Check("GetTrackId", r.Status); err != nil {
		return 0, err
	}
	return r.Id, nil
}

// ProcessorID looks a processor up by name.
func (c *Client) ProcessorID(name string) (int32, error) {
	ctx, cancel := Ctx()
	defer cancel()
	r, err := c.Graph.GetProcessorId(ctx, &rpc.GenericStringValue{Value: name})
	if err != nil {
		return 0, err
	}
	if err := Check("GetProcessorId", r.Status); err != nil {
		return 0, err
	}
	return r.Id, nil
}

// WaitFor polls fn until it succeeds or the timeout expires. Used after async creations, where the
// completion notification can race the lookup.
func WaitFor[T any](timeout time.Duration, fn func() (T, error)) (T, error) {
	deadline := time.Now().Add(timeout)
	for {
		v, err := fn()
		if err == nil || time.Now().After(deadline) {
			return v, err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// IsNotFound reports whether err is a NOT_FOUND status.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == rpc.CommandStatus_NOT_FOUND
}
