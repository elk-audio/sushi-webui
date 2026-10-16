package core

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Meta is the WebUI-only state that Sushi doesn't store (ARCHITECTURE.md §4.5). Keyed by names,
// which survive Sushi restarts and session restores, unlike ids.
type Meta struct {
	Version int                   `json:"version"`
	Project string                `json:"project"`
	Tracks  map[string]*TrackMeta `json:"tracks"`
	Procs   map[string]*ProcMeta  `json:"processors"`
	UI      UIState               `json:"ui"`
}

type TrackMeta struct {
	Kind   string               `json:"kind,omitempty"`
	Label  string               `json:"label,omitempty"` // user-chosen display name (Sushi can't rename tracks)
	Color  string               `json:"color,omitempty"`
	Order  int                  `json:"order"`
	Mute   bool                 `json:"mute"`
	Solo   bool                 `json:"solo"`
	Sends  map[string]*SendMeta `json:"sends,omitempty"`  // by bus name
	Macros map[string][]string  `json:"macros,omitempty"` // processor name -> parameter names
}

type SendMeta struct {
	DB   float32 `json:"db"`
	Post bool    `json:"post"`
}

type ProcMeta struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
	Path string `json:"path,omitempty"`
	Role string `json:"role,omitempty"`
}

func newMeta() *Meta {
	return &Meta{Version: 1, Project: "Untitled", Tracks: map[string]*TrackMeta{}, Procs: map[string]*ProcMeta{}}
}

func (m *Meta) track(name string) *TrackMeta {
	tm, ok := m.Tracks[name]
	if !ok {
		tm = &TrackMeta{Order: len(m.Tracks)}
		m.Tracks[name] = tm
	}
	if tm.Sends == nil {
		tm.Sends = map[string]*SendMeta{}
	}
	if tm.Macros == nil {
		tm.Macros = map[string][]string{}
	}
	return tm
}

func loadMeta(path string) *Meta {
	m := newMeta()
	raw, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	if err := json.Unmarshal(raw, m); err != nil {
		slog.Warn("ignoring unreadable metadata", "path", path, "err", err)
		return newMeta()
	}
	if m.Tracks == nil {
		m.Tracks = map[string]*TrackMeta{}
	}
	if m.Procs == nil {
		m.Procs = map[string]*ProcMeta{}
	}
	return m
}

func (m *Meta) save(path string) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ---------------------------------------------------------------- projects on disk

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9 _.-]+`)

func projectDir(dataDir, name string) (string, error) {
	clean := strings.TrimSpace(unsafeName.ReplaceAllString(name, "_"))
	if clean == "" || clean == "." || clean == ".." {
		return "", errors.New("enter a project name using letters, numbers, spaces, - or _")
	}
	return filepath.Join(dataDir, "projects", clean+".sushiproj"), nil
}

func listProjects(dataDir string) []string {
	dirs, _ := filepath.Glob(filepath.Join(dataDir, "projects", "*.sushiproj"))
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		out = append(out, strings.TrimSuffix(filepath.Base(d), ".sushiproj"))
	}
	sort.Strings(out)
	return out
}
