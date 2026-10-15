package core

import (
	"math"

	"github.com/elk-audio/sushi-webui/internal/catalog"
)

// JSON shapes follow docs/PROTOCOL.md exactly.

type Param struct {
	ID          int32   `json:"id"`
	Name        string  `json:"name"`
	Label       string  `json:"label"`
	Unit        string  `json:"unit"`
	Type        string  `json:"type"` // float | int | bool
	Automatable bool    `json:"automatable"`
	Min         float32 `json:"min"`
	Max         float32 `json:"max"`
	N           float32 `json:"n"`
	Txt         string  `json:"txt"`
}

// Domain converts the normalized value to the domain range (Sushi maps linearly for float params).
func (p *Param) Domain() float32 { return p.Min + p.N*(p.Max-p.Min) }

type Prop struct {
	ID    int32  `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type Proc struct {
	ID       int32        `json:"id"`
	Name     string       `json:"name"`
	Label    string       `json:"label"`
	UID      string       `json:"uid"`
	Type     string       `json:"type"`
	Role     catalog.Role `json:"role"`
	Bypass   bool         `json:"bypass"`
	Program  int32        `json:"program"`
	Programs []string     `json:"programs"`
	Params   []*Param     `json:"params"`
	Props    []*Prop      `json:"props"`
	Macros   []int32      `json:"macros"`
	Path     string       `json:"-"`
}

type Send struct {
	Bus     int32   `json:"bus"`
	BusName string  `json:"busName"`
	DB      float32 `json:"db"`
	Post    bool    `json:"post"`

	proc      int32 // hidden sushi.testing.send processor
	gainParam int32
	written   float32 // last gain (dB) written to Sushi, NaN if unknown
}

type Track struct {
	ID       int32   `json:"id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"` // audio | midi | bus | pre | post
	Color    string  `json:"color"`
	Channels int     `json:"channels"`
	Gain     *Param  `json:"gain"`
	Pan      *Param  `json:"pan"`
	Mute     bool    `json:"mute"`
	Solo     bool    `json:"solo"`
	Input    string  `json:"input"`
	Output   string  `json:"output"`
	Chain    []*Proc `json:"chain"`
	Sends    []*Send `json:"sends"`
	Meter    int     `json:"meter"`

	muteParam  int32
	sushiMute  bool // mute value currently in Sushi (user mute OR implied by solo)
	meterProc  int32
	returnProc int32
	returnName string
	hidden     []int32 // hidden processor ids in chain order (sends, meter), used for insert positions
}

func (t *Track) gainDb() float32 {
	if t.Gain == nil {
		return 0
	}
	return t.Gain.Domain()
}

// chGainsDb mirrors Track::_apply_pan_and_gain (src/engine/track.cpp:32-54,380): returns the
// per-channel gain in dB that Sushi applies after the processor chain.
func (t *Track) chGainsDb() (float32, float32) {
	g := float64(math.Pow(10, float64(t.gainDb())/20))
	if t.Pan == nil {
		db := float32(20 * math.Log10(math.Max(g, 1e-9)))
		return db, db
	}
	const pan3db = 1.412537
	pan := float64(t.Pan.Domain())
	var l, r float64
	if pan < 0 {
		l = g * (1 + pan - pan3db*pan)
		r = g * (1 + pan)
	} else {
		l = g * (1 - pan)
		r = g * (1 - pan + pan3db*pan)
	}
	l, r = l/pan3db, r/pan3db
	return float32(20 * math.Log10(math.Max(l, 1e-9))), float32(20 * math.Log10(math.Max(r, 1e-9)))
}

type SushiInfo struct {
	Connected  bool    `json:"connected"`
	Version    string  `json:"version"`
	Inputs     int     `json:"inputs"`
	Outputs    int     `json:"outputs"`
	MidiInputs int     `json:"midiInputs"`
	Samplerate float32 `json:"samplerate"`
}

type ProjectInfo struct {
	Name  string   `json:"name"`
	Dirty bool     `json:"dirty"`
	List  []string `json:"list"`
}

type State struct {
	Sushi     SushiInfo       `json:"sushi"`
	Project   ProjectInfo     `json:"project"`
	UI        UIState         `json:"ui"`
	Transport TransportState  `json:"transport"`
	Cpu       float32         `json:"cpu"`
	Tracks    []*Track        `json:"tracks"`
	Pre       *Track          `json:"pre"`
	Master    *Track          `json:"master"`
	Catalog   []catalog.Entry `json:"catalog"`
}

type UIState struct {
	Advanced bool `json:"advanced"`
}

type TransportState struct {
	Tempo   float32 `json:"tempo"`
	Playing bool    `json:"playing"`
}

type pkey struct{ proc, param int32 }

var palette = []string{"#e0a33a", "#5aa0d8", "#c9719a", "#58b38b", "#9b86d9", "#d9734e", "#4fb3b8", "#b8b04f"}

// Reserved name prefixes of hidden processors (ARCHITECTURE.md §4.5).
const (
	prefixMeter  = "__meter__"
	prefixReturn = "__return__"
	prefixSend   = "__send__"
)
