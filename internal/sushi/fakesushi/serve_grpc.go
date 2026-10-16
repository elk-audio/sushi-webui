package fakesushi

import (
	"context"
	"net"
	"testing"

	"github.com/elk-audio/sushi-webui/internal/sushi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// ServeGRPC serves the fake over an in-memory gRPC connection and returns a Dialer for it.
func ServeGRPC(tb testing.TB, f *Fake) sushi.Dialer {
	tb.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	f.Register(srv)
	go func() { _ = srv.Serve(lis) }()
	tb.Cleanup(srv.Stop)
	return sushi.Dialer{Name: "fake grpc", Dial: func() (*sushi.Client, error) {
		conn, err := grpc.NewClient("passthrough:///fake",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return sushi.New(conn, conn), nil
	}}
}
