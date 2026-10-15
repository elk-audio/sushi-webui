// Command sushi-webui serves a responsive web UI for the Sushi plugin host.
package main

import (
	"context"
	"flag"
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
	"github.com/elk-audio/sushi-webui/web"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	home, _ := os.UserHomeDir()
	listen := flag.String("listen", ":8080", "HTTP listen address")
	sushiAddr := flag.String("sushi", "localhost:51051", "Sushi gRPC address")
	dataDir := flag.String("data-dir", filepath.Join(home, ".config", "sushi-webui"), "directory for projects and settings")
	webDir := flag.String("web-dir", "", "serve the frontend from this directory instead of the embedded build")
	dev := flag.Bool("dev", false, "development mode: accept WebSocket connections from any origin (Vite proxy)")
	logLevel := flag.String("log-level", "info", "debug | info | warn | error")
	var pluginPaths multiFlag
	flag.Var(&pluginPaths, "plugin-path", "extra folder to scan for VST3 and LV2 plugins (repeatable)")
	flag.Parse()

	var lvl slog.Level
	_ = lvl.UnmarshalText([]byte(*logLevel))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		slog.Error("can't create data dir", "dir", *dataDir, "err", err)
		os.Exit(1)
	}

	vst3 := []string{"/usr/lib/vst3", "/usr/local/lib/vst3", filepath.Join(home, ".vst3")}
	lv2 := []string{"/usr/lib/lv2", "/usr/local/lib/lv2", filepath.Join(home, ".lv2")}
	if runtime.GOOS == "darwin" {
		vst3 = append(vst3, "/Library/Audio/Plug-Ins/VST3", filepath.Join(home, "Library/Audio/Plug-Ins/VST3"))
		lv2 = append(lv2, "/Library/Audio/Plug-Ins/LV2", filepath.Join(home, "Library/Audio/Plug-Ins/LV2"))
	}
	if p := os.Getenv("LV2_PATH"); p != "" {
		lv2 = append(lv2, filepath.SplitList(p)...)
	}
	vst3 = append(vst3, pluginPaths...)
	lv2 = append(lv2, pluginPaths...)
	cat := catalog.Build(vst3, lv2, filepath.Join(*dataDir, "plugins.json"))

	eng := core.New(*sushiAddr, *dataDir, cat)
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
	slog.Info("sushi-webui listening", "addr", *listen, "sushi", *sushiAddr, "data", *dataDir)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server", "err", err)
		os.Exit(1)
	}
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
