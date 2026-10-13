package giztestcmd

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
)

const (
	replayMaxBytes  = 128 << 20
	replayMaxChunks = 200000
)

//go:embed recording.gohtml
var recordingTemplate string

//go:embed recording_player.js
var recordingPlayer string

// replayRecord retains only conversation media and lifecycle markers. RPC
// bodies and configuration are not dumped; known secret inputs are redacted.
// Its mutex owns bounded memory only; encoding and filesystem I/O happen after
// taking a snapshot, once the runner has joined its stream readers.
type replayRecord struct {
	directory           *playRecord
	mu                  sync.Mutex
	events              []replayEvent
	audio               map[string]*replayAudio
	tracks              []*replayAudio
	streams             map[string]int
	attempts            map[string]int
	bytes, chunks       int
	maxBytes, maxChunks int
	truncated           bool
	sealed              bool
	pages               []string
}

type replayEvent struct {
	At            time.Time `json:"-"`
	OffsetMS      float64   `json:"offset_ms"`
	Path          string    `json:"path"`
	TaskID        string    `json:"task_id"`
	Step          string    `json:"step"`
	Parent        string    `json:"parent,omitempty"`
	Cleanup       bool      `json:"cleanup,omitempty"`
	Attempt       int       `json:"attempt,omitempty"`
	Client        string    `json:"client,omitempty"`
	Direction     string    `json:"direction,omitempty"`
	Role          string    `json:"role,omitempty"`
	Stream        int       `json:"stream,omitempty"`
	Kind          string    `json:"kind"`
	MIME          string    `json:"mime,omitempty"`
	Text          string    `json:"text,omitempty"`
	Interim       bool      `json:"interim,omitempty"`
	BOS           bool      `json:"bos,omitempty"`
	EOS           bool      `json:"eos,omitempty"`
	Error         string    `json:"error,omitempty"`
	Audio         string    `json:"audio,omitempty"`
	OriginalAudio string    `json:"original_audio,omitempty"`
	AudioData     string    `json:"audio_data,omitempty"`
	DurationMS    float64   `json:"duration_ms,omitempty"`
	Packets       int       `json:"packets,omitempty"`
}

type replayAudio struct {
	event  int
	chunks [][]byte
}

type replayCapture struct {
	record     *replayRecord
	base       replayEvent
	key        string
	redactions []string
}

type replayData struct {
	Version   string         `json:"version"`
	Report    giztest.Report `json:"report"`
	Events    []replayEvent  `json:"events"`
	Truncated bool           `json:"truncated"`
	Errors    []string       `json:"errors,omitempty"`
}

func newReplayRecord(path string) (*replayRecord, error) {
	directory, err := newPlayRecord(path)
	if err != nil {
		return nil, err
	}
	return &replayRecord{directory: directory, audio: make(map[string]*replayAudio), streams: make(map[string]int), attempts: make(map[string]int), maxBytes: replayMaxBytes, maxChunks: replayMaxChunks}, nil
}

func (r *replayRecord) capture(path, taskID string, req giztest.StepRequest) *replayCapture {
	parent := ""
	if req.Parent != nil {
		parent = req.Parent.ID
	}
	key := fmt.Sprintf("%s\x00%s\x00%t\x00%s\x00%s", path, taskID, req.Cleanup, parent, req.Step.ID)
	r.mu.Lock()
	if r.truncated || r.sealed || len(r.attempts) >= r.maxChunks {
		r.truncated = r.truncated || !r.sealed
		r.mu.Unlock()
		return nil
	}
	r.attempts[key]++
	attempt := r.attempts[key]
	r.mu.Unlock()
	return &replayCapture{record: r, key: fmt.Sprintf("%s\x00%d", key, attempt), base: replayEvent{Path: path, TaskID: taskID, Step: req.Step.ID, Parent: parent, Cleanup: req.Cleanup, Attempt: attempt}}
}

func (r *replayRecord) observeOutput(ctx context.Context, req giztest.StepRequest, text string) {
	if r == nil {
		return
	}
	capture := r.capture(req.DocumentPath, giztest.TaskID(ctx), req)
	capture.observe("", "output", &genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{Label: "output"}}, time.Now())
}

func (c *replayCapture) observeSpeech(step giztest.Step, request any, result operationResult) {
	client := step.Client
	if object, ok := request.(map[string]any); ok && step.Speech.Method == "server.speech.synthesize" {
		if text, ok := object["text"].(string); ok {
			c.observe(client, "prepared", &genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{Label: "speech"}}, time.Now())
		}
	}
	if value, ok := result.assertion.(map[string]any); ok {
		if text, ok := value["transcript"].(string); ok {
			c.observe(client, "received", &genx.MessageChunk{Part: genx.Text(text), Ctrl: &genx.StreamCtrl{Label: "transcript"}}, time.Now())
		}
		if audio, ok := result.saved.([]byte); ok {
			if mime, ok := value["content_type"].(string); ok {
				// Speech synthesis returns Opus in Ogg while its RPC metadata
				// names only the container. Relay streams require the codec.
				if mime == "audio/ogg" {
					mime = "audio/ogg;codecs=opus"
				}
				c.observe(client, "prepared", &genx.MessageChunk{Part: &genx.Blob{MIMEType: mime, Data: audio}, Ctrl: &genx.StreamCtrl{Label: "speech"}}, time.Now())
			}
		}
	}
}

func (c *replayCapture) observe(client, direction string, chunk *genx.MessageChunk, at time.Time) {
	if c == nil || chunk == nil {
		return
	}
	event := c.base
	event.At, event.Client, event.Direction = at, client, direction
	event.Role = "assistant"
	if direction == "sent" {
		event.Role = "user"
	}
	streamID := ""
	event.MIME, _ = chunk.MIMEType()
	if ctrl := chunk.Ctrl; ctrl != nil {
		streamID = ctrl.StreamID
		if ctrl.Label != "" {
			event.Role = ctrl.Label
		}
		event.BOS, event.EOS, event.Interim = ctrl.BeginOfStream, ctrl.EndOfStream, ctrl.TextInterim
		if ctrl.Error != "" || ctrl.ErrorCode != "" {
			event.Error = giztest.SafeError(fmt.Errorf("%s %s", ctrl.ErrorCode, ctrl.Error), c.redactions...)
		}
	}
	var audio []byte
	switch part := chunk.Part.(type) {
	case genx.Text:
		event.Kind, event.Text = "text", string(part)
		for _, secret := range c.redactions {
			event.Text = strings.ReplaceAll(event.Text, secret, "[REDACTED]")
		}
	case *genx.Blob:
		if part != nil && len(part.Data) > 0 && relayOpusMIME(part.MIMEType) {
			event.Kind, audio = "audio", part.Data
		}
	}
	if event.Kind == "" || (event.Kind == "text" && event.Text == "") {
		event.Kind = "control"
		if !event.BOS && !event.EOS && event.Error == "" {
			return
		}
	}
	key := c.key + "\x00" + client + "\x00" + direction + "\x00" + streamID
	c.record.appendEvent(event, key, audio)
}

func (r *replayRecord) appendEvent(event replayEvent, key string, audio []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sealed || r.truncated {
		return
	}
	if r.chunks >= r.maxChunks || len(audio)+len(event.Text) > r.maxBytes-r.bytes {
		r.truncated = true
		return
	}
	r.chunks++
	r.bytes += len(audio) + len(event.Text)
	stream, ok := r.streams[key]
	if !ok {
		stream = len(r.streams) + 1
		r.streams[key] = stream
	}
	event.Stream = stream
	// A text EOS does not close the accompanying audio channel.
	audioKey := key + "\x00audio"
	if len(audio) > 0 {
		track := r.audio[audioKey]
		if track == nil {
			track = &replayAudio{event: len(r.events)}
			r.audio[audioKey] = track
			r.tracks = append(r.tracks, track)
			r.events = append(r.events, event)
		}
		track.chunks = append(track.chunks, bytes.Clone(audio))
		if event.EOS {
			delete(r.audio, audioKey)
		}
		return
	}
	if event.EOS && relayOpusMIME(event.MIME) {
		delete(r.audio, audioKey)
	}
	r.events = append(r.events, event)
}

type replayPeerStream struct {
	peerStream
	client  string
	capture atomic.Pointer[replayCapture]
}

func bindReplayCapture(stream peerStream, capture *replayCapture) {
	switch stream := stream.(type) {
	case *replayPeerStream:
		stream.capture.Store(capture)
	case *packetTimedPeerStream:
		bindReplayCapture(stream.peerStream, capture)
	}
}

func (c *replayCapture) wrap(client string, stream peerStream) peerStream {
	if c == nil {
		return stream
	}
	wrapped := &replayPeerStream{peerStream: stream, client: client}
	wrapped.capture.Store(c)
	return wrapped
}

func (s *replayPeerStream) Push(ctx context.Context, chunk *genx.MessageChunk) error {
	capture := s.capture.Load()
	if err := s.peerStream.Push(ctx, chunk); err != nil {
		return err
	}
	capture.observe(s.client, "sent", chunk, time.Now())
	return nil
}

func (s *replayPeerStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.peerStream.Next()
	if chunk != nil {
		s.capture.Load().observe(s.client, "received", chunk, time.Now())
	}
	return chunk, err
}

func (r *replayRecord) snapshot() (replayData, []*replayAudio) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sealed = true
	return replayData{Version: "v1", Events: slices.Clone(r.events), Truncated: r.truncated}, slices.Clone(r.tracks)
}

func (r *replayRecord) commit(report giztest.Report) error {
	data, tracks := r.snapshot()
	data.Report = report
	// Preserve acceptance results even if a later media export fails.
	if err := r.directory.commit(report, nil); err != nil {
		return err
	}
	for i := range data.Events {
		data.Events[i].OffsetMS = max(0, float64(data.Events[i].At.Sub(report.StartedAt))/float64(time.Millisecond))
	}
	exportErr := writeTestReplay(r.directory.target, data, tracks)
	r.pages = []string{"index.html"}
	if data.Truncated {
		exportErr = errors.Join(exportErr, fmt.Errorf("recording is incomplete: exceeded %d bytes or %d chunks; partial recording saved", r.maxBytes, r.maxChunks))
	}
	return exportErr
}

func writeTestReplay(directory string, data replayData, tracks []*replayAudio) error {
	if err := giztest.WriteReport(filepath.Join(directory, "report.json"), data.Report); err != nil {
		return err
	}
	if len(tracks) > 0 {
		if err := os.Mkdir(filepath.Join(directory, "audio"), 0o700); err != nil {
			return fmt.Errorf("create replay audio directory: %w", err)
		}
	}
	for _, track := range tracks {
		event := &data.Events[track.event]
		name := fmt.Sprintf("audio/%06d.ogg", track.event+1)
		packets, err := replayPackets(track.chunks)
		if err == nil {
			err = writePlayAudio(filepath.Join(directory, filepath.FromSlash(name)), packets)
		}
		if err != nil {
			event.Error = giztest.SafeError(fmt.Errorf("export audio: %w", err))
			data.Errors = append(data.Errors, event.Error)
			continue
		}
		event.Audio, event.Packets = name, len(packets)
		for _, packet := range packets {
			event.DurationMS += float64(codecconv.OpusPacketRTPTicks(packet)) / 48
		}
		// WAV plays directly from file://; the original Opus is also retained.
		wavName := strings.TrimSuffix(name, ".ogg") + ".wav"
		if opus.IsRuntimeSupported() {
			if err := writeReplayWAV(filepath.Join(directory, filepath.FromSlash(wavName)), packets); err != nil {
				event.Error = giztest.SafeError(fmt.Errorf("export WAV: %w", err))
				data.Errors = append(data.Errors, event.Error)
			} else {
				event.Audio, event.OriginalAudio = wavName, name
			}
		}
		media, err := os.ReadFile(filepath.Join(directory, filepath.FromSlash(event.Audio)))
		if err != nil {
			return fmt.Errorf("embed recorded audio: %w", err)
		}
		mediaType := "audio/wav"
		if event.OriginalAudio == "" {
			mediaType = "audio/ogg"
		}
		event.AudioData = "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(media)
	}
	slices.SortStableFunc(data.Events, func(a, b replayEvent) int { return a.At.Compare(b.At) })
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay: %w", err)
	}
	page, err := renderRecording(data)
	if err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
	}{
		{"replay.json", encoded},
		{"transcript.txt", []byte(replayTranscript(data))},
		{"index.html", page},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(directory, file.name), file.data, 0o600); err != nil {
			return fmt.Errorf("write replay %s: %w", file.name, err)
		}
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync replay: %w", err)
	}
	if len(data.Errors) > 0 {
		return fmt.Errorf("replay is incomplete: %d audio exports failed; partial replay saved", len(data.Errors))
	}
	return nil
}

func renderRecording(data replayData) ([]byte, error) {
	page, err := template.New("recording").Parse(recordingTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse recording template: %w", err)
	}
	title := "Giztest 测试记录"
	if len(data.Report.Tasks) > 0 {
		title = data.Report.Tasks[0].Name
	}
	var out bytes.Buffer
	// Player is trusted source embedded in this binary. Data is a typed value;
	// html/template serializes and escapes it for the JSON script context.
	if err := page.Execute(&out, struct {
		Title  string
		Data   replayData
		Player template.JS
	}{title, data, template.JS(recordingPlayer)}); err != nil {
		return nil, fmt.Errorf("render recording: %w", err)
	}
	return out.Bytes(), nil
}

func writeReplayWAV(path string, packets [][]byte) (err error) {
	decoder, err := opus.NewDecoder(16000, 1)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, decoder.Close()) }()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	header := pcm.L16Mono16K.WAV(nil)
	if _, err := file.Write(header); err != nil {
		return err
	}
	var total uint32
	for _, packet := range packets {
		samples, err := decoder.Decode(packet, 1920, false)
		if err != nil {
			return err
		}
		if uint64(total)+uint64(len(samples)*2) > (1<<32)-44 {
			return errors.New("decoded audio exceeds WAV size limit")
		}
		if err := binary.Write(file, binary.LittleEndian, samples); err != nil {
			return err
		}
		total += uint32(len(samples) * 2)
	}
	binary.LittleEndian.PutUint32(header[4:8], 36+total)
	binary.LittleEndian.PutUint32(header[40:44], total)
	if _, err := file.WriteAt(header, 0); err != nil {
		return err
	}
	return file.Sync()
}

func replayPackets(chunks [][]byte) ([][]byte, error) {
	var packets [][]byte
	for _, chunk := range chunks {
		decoded, err := decodeOpusPackets(chunk)
		if err != nil {
			return nil, err
		}
		packets = append(packets, decoded...)
	}
	if len(packets) == 0 {
		return nil, errors.New("no Opus packets")
	}
	return packets, nil
}

func replayTranscript(data replayData) string {
	var out strings.Builder
	fmt.Fprintf(&out, "Giztest %s · %s\n", data.Report.Status, data.Report.StartedAt.Format(time.RFC3339))
	textByTask := make(map[string][]replayEvent)
	for _, event := range joinedReplayText(data.Events) {
		key := event.Path + "\x00" + event.TaskID
		textByTask[key] = append(textByTask[key], event)
	}
	for _, task := range data.Report.Tasks {
		fmt.Fprintf(&out, "\n%s · %s · %s\n", task.Name, task.TaskID, task.Status)
		if task.Error != "" {
			fmt.Fprintln(&out, task.Error)
		}
		for _, event := range textByTask[task.Path+"\x00"+task.TaskID] {
			if event.Text != "" {
				interim := ""
				if event.Interim {
					interim = " (interim)"
				}
				fmt.Fprintf(&out, "[%0.3fs] %s #%d %s %s %s%s: %s\n", event.OffsetMS/1000, event.Step, event.Attempt, event.Client, event.Direction, event.Role, interim, event.Text)
			}
		}
	}
	if data.Truncated || len(data.Errors) > 0 {
		fmt.Fprintln(&out, "\nRecording incomplete; see replay.json for details.")
	}
	return out.String()
}

func joinedReplayText(events []replayEvent) []replayEvent {
	var joined []replayEvent
	var texts []*strings.Builder
	indices := make(map[string]int)
	for _, event := range events {
		if event.Text == "" {
			continue
		}
		key := fmt.Sprintf("%d\x00%s\x00%t", event.Stream, event.Role, event.Interim)
		if index, ok := indices[key]; ok {
			texts[index].WriteString(event.Text)
		} else {
			indices[key] = len(joined)
			joined = append(joined, event)
			var text strings.Builder
			text.WriteString(event.Text)
			texts = append(texts, &text)
		}
	}
	for i := range joined {
		joined[i].Text = texts[i].String()
	}
	return joined
}
