package sushi

import (
	"fmt"

	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Dialer opens a new connection to Sushi. The engine calls it at the start of every session, so a
// Dialer is the only place that knows which transport is in use.
type Dialer struct {
	// Name describes the target for logs, e.g. "grpc localhost:51051".
	Name string
	Dial func() (*Client, error)
}

// GRPCDialer connects over gRPC. The connection is lazy; the first call reveals failures.
func GRPCDialer(addr string) Dialer {
	return Dialer{Name: "grpc " + addr, Dial: func() (*Client, error) {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return New(conn, conn), nil
	}}
}

// ZMQDialer connects over Sushi's ZeroMQ interface (ipc sockets on the local machine).
func ZMQDialer(cfg zmqconn.Config) Dialer {
	if cfg.Control == "" {
		cfg.Control = zmqconn.DefaultControl
	}
	if cfg.Notify == "" {
		cfg.Notify = zmqconn.DefaultNotify
	}
	return Dialer{Name: fmt.Sprintf("zmq %s + %s", cfg.Control, cfg.Notify), Dial: func() (*Client, error) {
		conn, err := zmqconn.Dial(cfg)
		if err != nil {
			return nil, err
		}
		return New(conn, conn), nil
	}}
}
