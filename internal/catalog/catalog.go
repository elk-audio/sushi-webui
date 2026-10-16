// Package catalog builds the list of plugins the user can add. Sushi has no plugin enumeration RPC,
// so the list comes from a predefined, categorized JSON file (vendor → category → plugin, the format
// of sushi-gui's installed_plugins.json). At startup every external plugin is checked against the
// plugin folders and dropped when it isn't installed. Internal plugins are always offered.
package catalog

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Role string

const (
	AudioFx    Role = "audio_fx"
	MidiFx     Role = "midi_fx"
	Instrument Role = "instrument"
)

type Entry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`   // top level of the hierarchy ("Internal", "Brickworks", …)
	Category string `json:"category"` // second level ("Reverb", …)
	UID      string `json:"uid"`
	Path     string `json:"path"`
	Type     string `json:"type"` // internal | vst2 | vst3 | lv2
	Role     Role   `json:"role"`
}

//go:embed plugins.json
var defaultList []byte

// Catalog is immutable after Build.
type Catalog struct {
	Entries []Entry
	byID    map[string]*Entry
	byUID   map[string]*Entry
}

func (c *Catalog) Get(id string) (*Entry, bool) { e, ok := c.byID[id]; return e, ok }

// ByUID finds an entry from a processor's uid (e.g. from a saved session).
func (c *Catalog) ByUID(uid string) (*Entry, bool) { e, ok := c.byUID[uid]; return e, ok }

// Visible returns the entries offered in the plugin browser, in list order.
func (c *Catalog) Visible() []Entry { return c.Entries }

type listItem struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
	Path string `json:"path"`
	Type string `json:"type"` // omitted for internal plugins; lv2 | vst2x | vst3x
	Role Role   `json:"role"` // optional; defaults from the category
}

// Build reads the plugin list (listPath, or the embedded default when empty) and keeps the plugins
// that are present. vst2Dirs, vst3Dirs and lv2Dirs may include non-existent paths.
func Build(listPath string, vst2Dirs, vst3Dirs, lv2Dirs []string) *Catalog {
	raw := defaultList
	if listPath != "" {
		b, err := os.ReadFile(listPath)
		if err != nil {
			slog.Warn("can't read plugin list, using the built-in one", "path", listPath, "err", err)
		} else {
			raw = b
		}
	}
	c := &Catalog{byID: map[string]*Entry{}, byUID: map[string]*Entry{}}
	lv2 := lv2URIs(lv2Dirs)
	var missing int
	walk(raw, func(vendor, category, key string, it listItem) {
		e := Entry{Name: it.Name, Vendor: vendor, Category: category, UID: it.UID, Path: it.Path, Role: it.Role}
		if e.Name == "" {
			e.Name = key
		}
		if e.Role == "" {
			e.Role = roleFor(category)
		}
		switch it.Type {
		case "", "internal":
			e.Type = "internal"
			e.ID = "int:" + it.UID
		case "lv2":
			e.Type, e.UID = "lv2", it.Path
			e.Path = ""
			e.ID = "lv2:" + it.Path
			if !lv2[it.Path] {
				missing++
				return
			}
		case "vst2x", "vst2":
			e.Type = "vst2"
			p := find(vst2Dirs, it.Path)
			if p == "" {
				missing++
				return
			}
			e.Path, e.UID = p, ""
			e.ID = "vst2:" + it.Path
		case "vst3x", "vst3":
			e.Type = "vst3"
			p := find(vst3Dirs, it.Path)
			if p == "" {
				missing++
				return
			}
			e.Path = p
			if e.UID == "" {
				e.UID = e.Name
			}
			e.ID = "vst3:" + it.Path + ":" + e.UID
		default:
			slog.Warn("unknown plugin type in list", "plugin", key, "type", it.Type)
			return
		}
		c.Entries = append(c.Entries, e)
	})
	c.index()
	slog.Info("plugin catalog built", "available", len(c.Entries), "not installed", missing)
	return c
}

func roleFor(category string) Role {
	switch category {
	case "MIDI & Control":
		return MidiFx
	case "Generator":
		return Instrument
	}
	return AudioFx
}

func (c *Catalog) index() {
	for i := range c.Entries {
		e := &c.Entries[i]
		c.byID[e.ID] = e
		if _, dup := c.byUID[e.UID]; !dup && e.UID != "" {
			c.byUID[e.UID] = e
		}
	}
}

// find returns the absolute path of name in the first folder that has it.
func find(dirs []string, name string) string {
	if name == "" {
		return ""
	}
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return ""
}

// ---------------------------------------------------------------- list file (order preserving)

// walk visits {"plugins": {vendor: {category: {key: item}}}} in file order.
func walk(raw []byte, fn func(vendor, category, key string, it listItem)) {
	var top struct {
		Plugins json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &top); err != nil {
		slog.Warn("invalid plugin list", "err", err)
		return
	}
	for _, v := range entries(top.Plugins) {
		for _, c := range entries(v.val) {
			for _, p := range entries(c.val) {
				var it listItem
				if err := json.Unmarshal(p.val, &it); err != nil {
					slog.Warn("invalid plugin list entry", "plugin", p.key, "err", err)
					continue
				}
				fn(v.key, c.key, p.key, it)
			}
		}
	}
}

type kv struct {
	key string
	val json.RawMessage
}

// entries returns the members of a JSON object in document order.
func entries(raw json.RawMessage) []kv {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out []kv
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			break
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			break
		}
		out = append(out, kv{k.(string), v})
	}
	return out
}

// ---------------------------------------------------------------- LV2 (minimal Turtle reading)

var reSubject = regexp.MustCompile(`^<([^>]+)>`)

// lv2URIs returns the URIs of the LV2 plugins found in the given folders.
func lv2URIs(dirs []string) map[string]bool {
	out := map[string]bool{}
	for _, dir := range dirs {
		bundles, _ := filepath.Glob(filepath.Join(dir, "*.lv2"))
		for _, b := range bundles {
			files, _ := filepath.Glob(filepath.Join(b, "*.ttl"))
			for _, f := range files {
				fh, err := os.Open(f)
				if err != nil {
					continue
				}
				sc := bufio.NewScanner(fh)
				sc.Buffer(make([]byte, 1<<20), 1<<20)
				var subject string
				for sc.Scan() {
					line := strings.TrimSpace(sc.Text())
					if m := reSubject.FindStringSubmatch(line); m != nil {
						subject = m[1]
					}
					if subject != "" && strings.Contains(subject, ":") &&
						(strings.Contains(line, "lv2:Plugin") || strings.Contains(line, "Plugin ,") || strings.Contains(line, "lv2:InstrumentPlugin")) {
						out[subject] = true
					}
				}
				fh.Close()
			}
		}
	}
	return out
}
