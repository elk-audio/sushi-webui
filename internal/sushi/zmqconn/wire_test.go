package zmqconn_test

import (
	"testing"

	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
)

// The numbers below come from reading Sushi's rpc_interface (protoc-gen-zmq): the wire ids are
// declaration order in sushi_rpc.proto. If the proto we compile against drifts from Sushi's, these
// fail instead of silently calling the wrong endpoint.
func TestWireIDsMatchSushi(t *testing.T) {
	cases := []struct {
		method            string
		service, endpoint int32
	}{
		{"/sushi_rpc.SystemController/GetSushiVersion", 0, 0},
		{"/sushi_rpc.SystemController/GetOutputAudioChannelCount", 0, 4},
		{"/sushi_rpc.TransportController/GetSamplerate", 1, 0},
		{"/sushi_rpc.TransportController/GetTempo", 1, 4},
		{"/sushi_rpc.TransportController/SetTempo", 1, 5},
		{"/sushi_rpc.TransportController/SetPlayingMode", 1, 6},
		{"/sushi_rpc.AudioGraphController/GetAllTracks", 4, 1},
		{"/sushi_rpc.AudioGraphController/GetTrackId", 4, 2},
		{"/sushi_rpc.AudioGraphController/GetProcessorId", 4, 5},
		{"/sushi_rpc.AudioGraphController/CreateTrack", 4, 11},
		{"/sushi_rpc.AudioGraphController/CreatePostTrack", 4, 14},
		{"/sushi_rpc.AudioGraphController/CreateProcessorOnTrack", 4, 15},
		{"/sushi_rpc.AudioGraphController/DeleteTrack", 4, 18},
		{"/sushi_rpc.ParameterController/GetParameterValue", 6, 4},
		{"/sushi_rpc.ParameterController/SetParameterValue", 6, 7},
		{"/sushi_rpc.ParameterController/SetPropertyValue", 6, 13},
		{"/sushi_rpc.SessionController/SaveSession", 11, 0},
		{"/sushi_rpc.SessionController/RestoreSession", 11, 1},
		{"/sushi_rpc.NotificationController/SubscribeToTransportChanges", 12, 0},
		{"/sushi_rpc.NotificationController/SubscribeToParameterUpdates", 12, 4},
		{"/sushi_rpc.NotificationController/SubscribeToAsyncCommandUpdates", 12, 6},
	}
	for _, c := range cases {
		s, e, err := zmqconn.Lookup(c.method)
		if err != nil || s != c.service || e != c.endpoint {
			t.Errorf("%s = (%d,%d,%v), want (%d,%d)", c.method, s, e, err, c.service, c.endpoint)
		}
	}
}

func TestNotificationCodesMatchSushi(t *testing.T) {
	for name, want := range map[string]int32{
		"sushi_rpc.AsyncCommandResponse": 13,
		"sushi_rpc.CpuTimings":           28,
		"sushi_rpc.ParameterUpdate":      59,
		"sushi_rpc.PropertyValue":        64,
		"sushi_rpc.TransportUpdate":      97,
		"sushi_rpc.TrackUpdate":          98,
		"sushi_rpc.ProcessorUpdate":      99,
	} {
		if got, ok := zmqconn.MessageCode(name); !ok || got != want {
			t.Errorf("%s = %d (%v), want %d", name, got, ok, want)
		}
	}
}

func TestCvGateIsRefused(t *testing.T) {
	// Sushi dereferences a null pointer on service 9; the client must never send it.
	if _, _, err := zmqconn.Lookup("/sushi_rpc.CvGateController/GetCvInputChannelCount"); err == nil {
		t.Fatal("CvGateController must be refused client-side")
	}
	if _, _, err := zmqconn.Lookup("/sushi_rpc.Nope/Nothing"); err == nil {
		t.Fatal("unknown method must fail")
	}
}
