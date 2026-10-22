// Command sushi-webui serves a responsive web UI for the Sushi plugin host.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/elk-audio/sushi-webui/internal/catalog"
	"github.com/elk-audio/sushi-webui/internal/core"
	"github.com/elk-audio/sushi-webui/internal/hub"
	"github.com/elk-audio/sushi-webui/internal/sushi"
	"github.com/elk-audio/sushi-webui/internal/sushi/zmqconn"
	"github.com/elk-audio/sushi-webui/web"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	home, _ := os.UserHomeDir()
	listen := flag.String("listen", ":8081", "HTTP listen address")
	transport := flag.String("transport", "grpc", "how to talk to Sushi: grpc | zmq (ZeroMQ, same machine only)")
	sushiAddr := flag.String("sushi", "localhost:51051", "Sushi gRPC address (--transport grpc)")
	zmqControl := flag.String("zmq-control", zmqconn.DefaultControl, "Sushi ZeroMQ control endpoint (--transport zmq)")
	zmqNotify := flag.String("zmq-notify", zmqconn.DefaultNotify, "Sushi ZeroMQ notification endpoint (--transport zmq)")
	dataDir := flag.String("data-dir", filepath.Join(home, ".config", "sushi-webui"), "directory for projects and settings")
	webDir := flag.String("web-dir", "", "serve the frontend from this directory instead of the embedded build")
	dev := flag.Bool("dev", false, "development mode: accept WebSocket connections from any origin (Vite proxy)")
	logLevel := flag.String("log-level", "info", "debug | info | warn | error")
	var pluginPaths multiFlag
	flag.Var(&pluginPaths, "plugin-path", "extra folder to look for the plugins in the list (repeatable)")
	pluginList := flag.String("plugin-list", "", "categorized plugin list (JSON, sushi-gui installed_plugins.json format); default: built-in list")
	audioDir := flag.String("audio-dir", filepath.Join(home, "Music"), "base folder for the Wav Streamer file picker (*.wav, searched 3 levels deep)")
	defIn := flag.Int("default-input", 1, "stereo input bus new tracks read from (1 = engine inputs 1-2, 2 = 3-4, ...)")
	defOut := flag.Int("default-output", 1, "stereo output bus new tracks play to (1 = engine outputs 1-2, 2 = 3-4, ...)")
	flag.Parse()
	if *defIn < 1 || *defOut < 1 {
		fmt.Fprintln(os.Stderr, "--default-input and --default-output are bus numbers starting at 1")
		os.Exit(2)
	}

	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(*logLevel))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		slog.Error("can't create data dir", "dir", *dataDir, "err", err)
		os.Exit(1)
	}

	vst2 := []string{"/usr/lib/vst", "/usr/local/lib/vst", filepath.Join(home, ".vst")}
	vst3 := []string{"/usr/lib/vst3", "/usr/local/lib/vst3", filepath.Join(home, ".vst3")}
	lv2 := []string{"/usr/lib/lv2", "/usr/local/lib/lv2", filepath.Join(home, ".lv2")}
	if runtime.GOOS == "darwin" {
		vst2 = append(vst2, "/Library/Audio/Plug-Ins/VST", filepath.Join(home, "Library/Audio/Plug-Ins/VST"))
		vst3 = append(vst3, "/Library/Audio/Plug-Ins/VST3", filepath.Join(home, "Library/Audio/Plug-Ins/VST3"))
		lv2 = append(lv2, "/Library/Audio/Plug-Ins/LV2", filepath.Join(home, "Library/Audio/Plug-Ins/LV2"))
	}
	if p := os.Getenv("LV2_PATH"); p != "" {
		lv2 = append(lv2, filepath.SplitList(p)...)
	}
	vst2 = append(vst2, pluginPaths...)
	vst3 = append(vst3, pluginPaths...)
	lv2 = append(lv2, pluginPaths...)
	cat := catalog.Build(*pluginList, vst2, vst3, lv2)

	var dialer sushi.Dialer
	switch *transport {
	case "grpc":
		dialer = sushi.GRPCDialer(*sushiAddr)
	case "zmq":
		dialer = sushi.ZMQDialer(zmqconn.Config{Control: *zmqControl, Notify: *zmqNotify})
	default:
		fmt.Fprintf(os.Stderr, "unknown --transport %q (want grpc or zmq)\n", *transport)
		os.Exit(2)
	}
	eng := core.New(dialer, *dataDir, cat)
	eng.SetAudioDir(*audioDir)
	eng.SetDefaultBuses(*defIn, *defOut)
	h := hub.New(eng, *dev)
	eng.SetSink(h)

	var static fs.FS
	if *webDir != "" {
		static = os.DirFS(*webDir)
	} else {
		static, _ = fs.Sub(web.Dist, "dist")
	}
	mux := http.NewServeMux()
	mux.Handle("/ws", h)
	mux.Handle("/", noCacheIndex(http.FileServerFS(static)))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go eng.Run(ctx)

	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	slog.Info("sushi-webui listening", "addr", *listen, "sushi", dialer.Name, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
	eng.FlushMeta()
}

// noCacheIndex keeps browsers from caching index.html across upgrades (assets are content-hashed).
func noCacheIndex(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}
