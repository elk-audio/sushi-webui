// Package catalog builds the list of plugins the user can add. Sushi has no plugin enumeration RPC,
// so this combines a built-in list of Sushi's internal plugins, a scan of VST3 and LV2 folders, and
// an optional user plugins.json override (see ARCHITECTURE.md §4.10).
package catalog

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Role string

const (
	AudioFx    Role = "audio_fx"
	MidiFx     Role = "midi_fx"
	Instrument Role = "instrument"
)

type Entry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	UID    string `json:"uid"`
	Path   string `json:"path"`
	Type   string `json:"type"` // internal | vst2 | vst3 | lv2
	Role   Role   `json:"role"`
	Hidden bool   `json:"-"`
}

type internalDef struct {
	uid, name string
	role      Role
}

// Internal plugins of Sushi 1.x (src/plugins). Utility plugins (peakmeter, send/return, cv, wav io)
// are deliberately not offered.
var internal = []internalDef{
	{"sushi.testing.arpeggiator", "Arpeggiator", MidiFx},
	{"sushi.testing.transposer", "Transposer", MidiFx},
	{"sushi.testing.step_sequencer", "Step Sequencer", MidiFx},
	{"sushi.brickworks.simple_synth", "Simple Synth", Instrument},
	{"sushi.testing.sampleplayer", "Sample Player", Instrument},
	{"sushi.testing.gain", "Gain", AudioFx},
	{"sushi.testing.equalizer", "Equalizer", AudioFx},
	{"sushi.testing.freeverb", "Freeverb", AudioFx},
	{"sushi.testing.sample_delay", "Sample Delay", AudioFx},
	{"sushi.testing.mono_summing", "Mono Summing", AudioFx},
	{"sushi.testing.stereo_mixer", "Stereo Mixer", AudioFx},
	{"sushi.brickworks.bitcrusher", "Bitcrusher", AudioFx},
	{"sushi.brickworks.cab_sim", "Cab Sim", AudioFx},
	{"sushi.brickworks.chorus", "Chorus", AudioFx},
	{"sushi.brickworks.clip", "Clip", AudioFx},
	{"sushi.brickworks.comb_delay", "Comb Delay", AudioFx},
	{"sushi.brickworks.compressor", "Compressor", AudioFx},
	{"sushi.brickworks.dist", "Distortion", AudioFx},
	{"sushi.brickworks.drive", "Drive", AudioFx},
	{"sushi.brickworks.eq3band", "EQ 3-band", AudioFx},
	{"sushi.brickworks.flanger", "Flanger", AudioFx},
	{"sushi.brickworks.fuzz", "Fuzz", AudioFx},
	{"sushi.brickworks.highpass", "Highpass", AudioFx},
	{"sushi.brickworks.multi_filter", "Multi Filter", AudioFx},
	{"sushi.brickworks.noise_gate", "Noise Gate", AudioFx},
	{"sushi.brickworks.notch", "Notch", AudioFx},
	{"sushi.brickworks.phaser", "Phaser", AudioFx},
	{"sushi.brickworks.ring_mod", "Ring Modulator", AudioFx},
	{"sushi.brickworks.saturation", "Saturation", AudioFx},
	{"sushi.brickworks.tremolo", "Tremolo", AudioFx},
	{"sushi.brickworks.vibrato", "Vibrato", AudioFx},
	{"sushi.brickworks.wah", "Wah", AudioFx},
}

// Catalog is immutable after Build.
type Catalog struct {
	Entries []Entry
	byID    map[string]*Entry
	byUID   map[string]*Entry
}

func (c *Catalog) Get(id string) (*Entry, bool) { e, ok := c.byID[id]; return e, ok }

// ByUID finds an entry from a processor's uid (e.g. from a saved session).
func (c *Catalog) ByUID(uid string) (*Entry, bool) { e, ok := c.byUID[uid]; return e, ok }

// Visible returns the entries offered in the plugin browser.
func (c *Catalog) Visible() []Entry {
	out := make([]Entry, 0, len(c.Entries))
	for _, e := range c.Entries {
		if !e.Hidden {
			out = append(out, e)
		}
	}
	return out
}

// Build scans the given folders. vst3Dirs and lv2Dirs may include non-existent paths.
func Build(vst3Dirs, lv2Dirs []string, overridePath string) *Catalog {
	c := &Catalog{byID: map[string]*Entry{}, byUID: map[string]*Entry{}}
	for _, d := range internal {
		c.Entries = append(c.Entries, Entry{ID: "int:" + d.uid, Name: d.name, Vendor: "Sushi", UID: d.uid, Type: "internal", Role: d.role})
	}
	var ext []Entry
	for _, dir := range vst3Dirs {
		ext = append(ext, scanVST3(dir)...)
	}
	for _, dir := range lv2Dirs {
		ext = append(ext, scanLV2(dir)...)
	}
	sort.Slice(ext, func(i, j int) bool { return strings.ToLower(ext[i].Name) < strings.ToLower(ext[j].Name) })
	c.Entries = append(c.Entries, ext...)
	if overridePath != "" {
		c.applyOverrides(overridePath)
	}
	c.index()
	slog.Info("plugin catalog built", "entries", len(c.Entries), "external", len(ext))
	return c
}

func (c *Catalog) index() {
	c.byID = map[string]*Entry{}
	c.byUID = map[string]*Entry{}
	for i := range c.Entries {
		e := &c.Entries[i]
		c.byID[e.ID] = e
		if _, dup := c.byUID[e.UID]; !dup {
			c.byUID[e.UID] = e
		}
	}
}

// ---------------------------------------------------------------- VST3

var trailingComma = regexp.MustCompile(`,(\s*[}\]])`)

type moduleInfo struct {
	Name    string `json:"Name"`
	Factory struct {
		Vendor string `json:"Vendor"`
	} `json:"Factory Info"`
	Classes []struct {
		Name          string   `json:"Name"`
		Category      string   `json:"Category"`
		Vendor        string   `json:"Vendor"`
		SubCategories []string `json:"Sub Categories"`
	} `json:"Classes"`
}

func scanVST3(dir string) []Entry {
	bundles, _ := filepath.Glob(filepath.Join(dir, "*.vst3"))
	var out []Entry
	for _, b := range bundles {
		abs, _ := filepath.Abs(b)
		info := filepath.Join(b, "Contents", "Resources", "moduleinfo.json")
		raw, err := os.ReadFile(info)
		if err != nil {
			// No moduleinfo: offer the bundle under its file name; Sushi accepts the class name as uid.
			name := strings.TrimSuffix(filepath.Base(b), ".vst3")
			out = append(out, Entry{ID: "vst3:" + name, Name: name, UID: name, Path: abs, Type: "vst3", Role: AudioFx})
			continue
		}
		var mi moduleInfo
		if err := json.Unmarshal(trailingComma.ReplaceAll(raw, []byte("$1")), &mi); err != nil {
			slog.Warn("unreadable moduleinfo.json", "path", info, "err", err)
			continue
		}
		for _, cl := range mi.Classes {
			if cl.Category != "Audio Module Class" {
				continue
			}
			role := AudioFx
			for _, s := range cl.SubCategories {
				if strings.Contains(strings.ToLower(s), "instrument") || strings.EqualFold(s, "Synth") {
					role = Instrument
				}
			}
			vendor := cl.Vendor
			if vendor == "" {
				vendor = mi.Factory.Vendor
			}
			out = append(out, Entry{ID: "vst3:" + cl.Name, Name: cl.Name, Vendor: vendor, UID: cl.Name, Path: abs, Type: "vst3", Role: role})
		}
	}
	return out
}

// ---------------------------------------------------------------- LV2 (minimal Turtle reading)

var (
	reSubject = regexp.MustCompile(`^<([^>]+)>`)
	reName    = regexp.MustCompile(`doap:name\s+"([^"]+)"`)
)

func scanLV2(dir string) []Entry {
	bundles, _ := filepath.Glob(filepath.Join(dir, "*.lv2"))
	var out []Entry
	for _, b := range bundles {
		files, _ := filepath.Glob(filepath.Join(b, "*.ttl"))
		var uri, name string
		role := AudioFx
		isPlugin := false
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
				if strings.Contains(line, "lv2:Plugin") || strings.Contains(line, "lv2:InstrumentPlugin") || strings.Contains(line, "Plugin ,") {
					if subject != "" && strings.Contains(subject, ":") && (uri == "" || uri == subject) {
						uri = subject
						isPlugin = true
					}
				}
				if strings.Contains(line, "lv2:InstrumentPlugin") {
					role = Instrument
				} else if strings.Contains(line, "lv2:MIDIPlugin") && role != Instrument {
					role = MidiFx
				}
				if m := reName.FindStringSubmatch(line); m != nil && name == "" && (subject == uri || subject == "") {
					name = m[1]
				}
			}
			fh.Close()
		}
		if !isPlugin || uri == "" {
			continue
		}
		if name == "" {
			name = strings.TrimSuffix(filepath.Base(b), ".lv2")
		}
		out = append(out, Entry{ID: "lv2:" + uri, Name: name, UID: uri, Type: "lv2", Role: role})
	}
	return out
}

// ---------------------------------------------------------------- overrides

type override struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	UID    string `json:"uid"`
	Path   string `json:"path"`
	Type   string `json:"type"`
	Role   Role   `json:"role"`
	Hidden bool   `json:"hidden"`
}

// plugins.json: {"plugins":[{"id":"vst3:mda JX10","role":"instrument"}, {"uid":"...","type":"vst2","path":"...","name":"..."}]}
func (c *Catalog) applyOverrides(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var f struct {
		Plugins []override `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		slog.Warn("ignoring invalid plugins.json", "path", path, "err", err)
		return
	}
	c.index()
	for _, o := range f.Plugins {
		if e, ok := c.byID[o.ID]; ok && o.ID != "" {
			if o.Name != "" {
				e.Name = o.Name
			}
			if o.Role != "" {
				e.Role = o.Role
			}
			e.Hidden = o.Hidden
			continue
		}
		if o.UID == "" || o.Type == "" {
			continue
		}
		id := o.ID
		if id == "" {
			id = o.Type + ":" + o.UID
		}
		role := o.Role
		if role == "" {
			role = AudioFx
		}
		name := o.Name
		if name == "" {
			name = o.UID
		}
		c.Entries = append(c.Entries, Entry{ID: id, Name: name, Vendor: o.Vendor, UID: o.UID, Path: o.Path, Type: o.Type, Role: role, Hidden: o.Hidden})
	}
}
