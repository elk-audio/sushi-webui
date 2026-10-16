package core

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	rpc "github.com/elk-audio/sushi-webui/internal/sushirpc"
)

const (
	uidWavStreamer = "sushi.testing.wav_streamer"
	maxWavDepth    = 3
	maxWavFiles    = 2000
)

// SetAudioDir sets the base folder offered to the Wav Streamer file picker. Empty disables it.
func (e *Engine) SetAudioDir(dir string) {
	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = abs
		}
	}
	e.audioDir = dir
}

// listWavs returns the *.wav files under the audio folder as slash-separated paths relative to it.
func (e *Engine) listWavs() []string {
	files := []string{}
	if e.audioDir == "" {
		return files
	}
	_ = filepath.WalkDir(e.audioDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(e.audioDir, p)
		if d.IsDir() {
			if rel != "." && (strings.Count(rel, string(filepath.Separator)) >= maxWavDepth || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".wav") && !strings.HasPrefix(d.Name(), ".") && len(files) < maxWavFiles {
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return strings.ToLower(files[i]) < strings.ToLower(files[j]) })
	return files
}

// setWavFile points a Wav Streamer at a file from the audio folder (rel is one of listWavs) and
// turns on its "playing" and "loop" parameters.
func (e *Engine) setWavFile(proc int32, rel string) error {
	if e.audioDir == "" {
		return userErr("no audio folder is configured (start the backend with --audio-dir)")
	}
	rel = filepath.FromSlash(rel)
	full := filepath.Join(e.audioDir, rel)
	if r, err := filepath.Rel(e.audioDir, full); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) ||
		!strings.EqualFold(filepath.Ext(full), ".wav") {
		return userErr("that isn't a wav file in the audio folder")
	}
	isWav := false
	if err := e.withProc(proc, func(_ *Track, p *Proc) { isWav = p.UID == uidWavStreamer }); err != nil {
		return err
	}
	if !isWav {
		return userErr("that plugin isn't a Wav Streamer")
	}
	// Ids come from Sushi, not from the mirror, which can lag right after the plugin was added.
	c := e.client()
	pid := &rpc.ProcessorIdentifier{Id: proc}
	fileProp, err := c.Param.GetPropertyId(ctxT(), &rpc.PropertyIdRequest{Processor: pid, PropertyName: "file"})
	if err != nil || fileProp.Id == nil {
		return userErr("the Wav Streamer has no file property")
	}
	if err := e.setProperty(proc, fileProp.Id.PropertyId, full); err != nil {
		return err
	}
	// Loop first: with "playing" on and loop still off, a file that fits in one block is treated as
	// finished and the plugin switches "playing" back off.
	for _, name := range []string{"loop", "playing"} {
		prm, err := c.Param.GetParameterId(ctxT(), &rpc.ParameterIdRequest{Processor: pid, ParameterName: name})
		if err != nil || prm.Id == nil {
			return userErr("the Wav Streamer has no %q parameter", name)
		}
		if _, err := c.Param.SetParameterValue(ctxT(), &rpc.ParameterValue{Parameter: prm.Id, Value: 1}); err != nil {
			return userErr("Sushi couldn't start playback: %v", err)
		}
	}
	// Sushi's notifications for these can be late or missing; keep the mirror in step.
	return e.withProc(proc, func(_ *Track, p *Proc) {
		for _, pm := range p.Params {
			if pm.Name == "loop" || pm.Name == "playing" {
				pm.N, pm.Txt = 1, "True"
			}
		}
	})
}
