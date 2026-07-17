package agenthost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkgs/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkgs/genx"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

const (
	historyEntryTypeGear  = "gear"
	historyEntryTypeAgent = "agent"

	historyOggOpusSampleRate = 48000
	historyOggOpusChannels   = 1
	historyReplayInterrupted = "interrupted"
	historyUpdatedLabel      = "workspace.history.updated"
	historyUpdatedDelay      = 25 * time.Millisecond
	defaultHistoryOutputKey  = "__default__"
)

type historyGearIDContextKey struct{}

func withHistoryGearID(ctx context.Context, gearID string) context.Context {
	gearID = strings.TrimSpace(gearID)
	if gearID == "" {
		return ctx
	}
	return context.WithValue(ctx, historyGearIDContextKey{}, gearID)
}

func historyGearID(ctx context.Context) string {
	value, _ := ctx.Value(historyGearIDContextKey{}).(string)
	return strings.TrimSpace(value)
}

func wrapHistoryAgent(agent Agent, history *workspace.HistoryStore) Agent {
	if agent == nil || history == nil {
		return agent
	}
	return &historyAgent{Agent: agent, history: history}
}

type historyAgent struct {
	Agent
	history *workspace.HistoryStore

	outputMu sync.Mutex
	outputs  map[string]*historyOutput
}

type historyOutput struct {
	output *genx.StreamBuilder

	replayMu       sync.Mutex
	replayCancel   context.CancelFunc
	replaySeq      uint64
	replayStreamID string
	replayRole     genx.Role
	replayName     string
	replayLabel    string

	forwardMu          sync.Mutex
	activeForward      map[historyForwardChunkKey]historyForwardRoute
	interruptedForward map[historyForwardChunkKey]struct{}

	notifyMu          sync.Mutex
	notifyTimer       *time.Timer
	notifyLastUpdated time.Time
}

type historyForwardRouteKey struct {
	streamID string
	label    string
}

type historyForwardRoute struct {
	role     genx.Role
	name     string
	streamID string
	label    string
}

type historyForwardChunkKey struct {
	historyForwardRouteKey
	mimeType string
}

func (a *historyAgent) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if a == nil || a.Agent == nil {
		return nil, fmt.Errorf("agenthost: history agent is nil")
	}
	outputKey := historyOutputKey(ctx)
	output := genx.NewGrowableStreamBuilder((&genx.ModelContextBuilder{}).Build(), 256)
	outputState := &historyOutput{output: output}
	a.outputMu.Lock()
	if a.outputs == nil {
		a.outputs = make(map[string]*historyOutput)
	}
	previous := a.outputs[outputKey]
	if previous != nil {
		previous.cancelReplay()
		previous.cancelHistoryUpdated()
	}
	a.outputs[outputKey] = outputState
	a.outputMu.Unlock()
	recorder := newHistoryRecorder(a.history, historyGearID(ctx), a.notifyHistoryUpdated)
	agentOutput, err := a.Agent.Transform(ctx, pattern, input)
	if err != nil {
		a.clearOutput(outputKey, outputState)
		_ = output.Abort(err)
		return nil, err
	}
	go a.forwardOutput(ctx, outputKey, outputState, agentOutput, output, recorder)
	return output.Stream(), nil
}

func (a *historyAgent) Status(ctx context.Context) (apitypes.PeerRunWorkspaceState, error) {
	state, err := a.Agent.Status(ctx)
	if err != nil {
		return state, err
	}
	available := true
	state.HistoryAvailable = &available
	return state, nil
}

func (a *historyAgent) ListHistory(ctx context.Context, req apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	if a == nil || a.history == nil {
		message := unsupportedMessage
		return apitypes.PeerRunHistoryListResponse{Available: false, Items: []apitypes.PeerRunHistoryEntry{}, HasNext: false, Message: &message}, nil
	}
	return a.history.List(ctx, req)
}

func (a *historyAgent) PlayHistory(ctx context.Context, req apitypes.PeerRunHistoryPlayRequest) (apitypes.PeerRunHistoryPlayResponse, error) {
	if a == nil || a.history == nil {
		message := unsupportedMessage
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unsupported", Message: &message}, nil
	}
	entry, err := a.history.Get(ctx, req.HistoryId)
	if err != nil {
		if historyIsNotExist(err) {
			message := "history entry not found"
			return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "not_found", Message: &message}, nil
		}
		return apitypes.PeerRunHistoryPlayResponse{}, err
	}
	if !entry.ReplayAvailable {
		message := "history entry has no replayable content"
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unsupported", Message: &message}, nil
	}
	outputState, streamID, ok := a.currentOutput(ctx)
	if !ok {
		message := "workspace history replay requires an active output stream"
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
	}
	chunks, err := a.replayChunks(ctx, streamID, entry)
	if err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
	}
	if len(chunks) == 0 {
		message := "history entry has no replayable content"
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "empty", Message: &message}, nil
	}
	role, name, label := historyReplayRoute(entry)
	if err := outputState.startReplay(streamID, role, name, label, chunks); err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
	}
	return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: req.HistoryId, State: "played"}, nil
}

func (a *historyAgent) forwardOutput(ctx context.Context, outputKey string, outputState *historyOutput, input genx.Stream, output *genx.StreamBuilder, recorder *historyRecorder) {
	defer input.Close()
	defer recorder.discard()
	for {
		if err := ctx.Err(); err != nil {
			_ = output.Abort(err)
			a.clearOutput(outputKey, outputState)
			return
		}
		chunk, err := input.Next()
		if err != nil {
			if flushErr := recorder.Flush(ctx); flushErr != nil {
				a.clearOutput(outputKey, outputState)
				_ = output.Abort(flushErr)
				return
			}
			a.clearOutput(outputKey, outputState)
			if IsStreamDone(err) {
				_ = output.Done(genx.Usage{})
				return
			}
			_ = output.Abort(err)
			return
		}
		if chunk == nil {
			continue
		}
		if historyOutputOnlyChunk(chunk) {
			if err := recorder.ObserveOutput(ctx, chunk); err != nil {
				a.clearOutput(outputKey, outputState)
				_ = output.Abort(err)
				return
			}
			continue
		}
		if outputState.observeForwardChunk(chunk) {
			continue
		}
		if err := recorder.ObserveOutput(ctx, chunk); err != nil {
			a.clearOutput(outputKey, outputState)
			_ = output.Abort(err)
			return
		}
		if err := output.Add(chunk.Clone()); err != nil {
			_ = recorder.Flush(ctx)
			a.clearOutput(outputKey, outputState)
			return
		}
	}
}

func historyOutputKey(ctx context.Context) string {
	gearID := historyGearID(ctx)
	if gearID == "" {
		return defaultHistoryOutputKey
	}
	return gearID
}

func (a *historyAgent) currentOutput(ctx context.Context) (*historyOutput, string, bool) {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	state := a.outputs[historyOutputKey(ctx)]
	if state == nil || state.output == nil {
		return nil, "", false
	}
	return state, fmt.Sprintf("history-replay-%d", time.Now().UnixNano()), true
}

func (a *historyAgent) notifyHistoryUpdated(lastUpdated time.Time) {
	if a == nil {
		return
	}
	a.outputMu.Lock()
	states := make([]*historyOutput, 0, len(a.outputs))
	for _, state := range a.outputs {
		if state != nil {
			states = append(states, state)
		}
	}
	a.outputMu.Unlock()
	for _, state := range states {
		state.notifyHistoryUpdated(lastUpdated)
	}
}

func (a *historyAgent) clearOutput(outputKey string, state *historyOutput) {
	clearReplay := false
	a.outputMu.Lock()
	if current := a.outputs[outputKey]; current == state {
		delete(a.outputs, outputKey)
		clearReplay = true
	}
	a.outputMu.Unlock()
	if clearReplay {
		state.cancelReplay()
		state.cancelHistoryUpdated()
		state.clearForwardOutput()
	}
}

func (o *historyOutput) clearForwardOutput() {
	if o == nil {
		return
	}
	o.forwardMu.Lock()
	defer o.forwardMu.Unlock()
	o.activeForward = nil
	o.interruptedForward = nil
}

func (a *historyAgent) replayChunks(ctx context.Context, streamID string, entry workspace.HistoryEntry) ([]*genx.MessageChunk, error) {
	role, name, label := historyReplayRoute(entry)
	var chunks []*genx.MessageChunk
	if strings.TrimSpace(entry.Text) != "" {
		chunks = append(chunks,
			historyTextChunk(role, name, streamID, label, entry.Text, false),
			historyTextChunk(role, name, streamID, label, "", true),
		)
	}
	for _, asset := range entry.Assets {
		if !isHistoryAudioMIME(asset.MIMEType) {
			continue
		}
		r, err := a.history.ReadAsset(ctx, asset.Name)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(r)
		closeErr := r.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return nil, err
		}
		audioChunks, err := historyAudioReplayChunks(role, name, streamID, label, asset.MIMEType, data)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, audioChunks...)
	}
	return chunks, nil
}

func historyReplayRoute(entry workspace.HistoryEntry) (genx.Role, string, string) {
	role := genx.RoleModel
	label := "assistant"
	if entry.Type == historyEntryTypeGear {
		role = genx.RoleUser
		label = "transcript"
	}
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = label
	}
	return role, name, label
}

func (o *historyOutput) observeForwardChunk(chunk *genx.MessageChunk) bool {
	if o == nil {
		return false
	}
	route, ok := historyForwardChunkRoute(chunk)
	if !ok {
		return false
	}
	o.forwardMu.Lock()
	defer o.forwardMu.Unlock()
	mimeType, hasMIME := historyForwardChunkMIME(chunk)
	if !hasMIME {
		if chunk.Part != nil || !chunk.IsEndOfStream() {
			return false
		}
		for key := range o.interruptedForward {
			if key.streamID == route.streamID {
				delete(o.interruptedForward, key)
			}
		}
		for key := range o.activeForward {
			if key.streamID == route.streamID {
				delete(o.activeForward, key)
			}
		}
		return false
	}
	key := historyForwardChunkKey{historyForwardRouteKey: route.key(), mimeType: mimeType}
	if _, interrupted := o.interruptedForward[key]; interrupted {
		if chunk.IsEndOfStream() {
			delete(o.interruptedForward, key)
		}
		return true
	}
	if chunk.IsEndOfStream() {
		delete(o.activeForward, key)
		return false
	}
	if o.activeForward == nil {
		o.activeForward = make(map[historyForwardChunkKey]historyForwardRoute)
	}
	o.activeForward[key] = route
	return false
}

func (o *historyOutput) interruptForwardOutput() []*genx.MessageChunk {
	if o == nil {
		return nil
	}
	o.forwardMu.Lock()
	defer o.forwardMu.Unlock()
	if len(o.activeForward) == 0 {
		return nil
	}
	interrupt := make([]*genx.MessageChunk, 0, len(o.activeForward))
	if o.interruptedForward == nil {
		o.interruptedForward = make(map[historyForwardChunkKey]struct{})
	}
	keys := make([]historyForwardChunkKey, 0, len(o.activeForward))
	for key := range o.activeForward {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].streamID != keys[j].streamID {
			return keys[i].streamID < keys[j].streamID
		}
		if keys[i].label != keys[j].label {
			return keys[i].label < keys[j].label
		}
		if keys[i].mimeType == keys[j].mimeType {
			return false
		}
		if keys[i].mimeType == "text/plain" {
			return true
		}
		if keys[j].mimeType == "text/plain" {
			return false
		}
		return keys[i].mimeType < keys[j].mimeType
	})
	for _, key := range keys {
		route := o.activeForward[key]
		interrupt = append(interrupt, historyForwardInterruptedChunk(route, key.mimeType))
		o.interruptedForward[key] = struct{}{}
		delete(o.activeForward, key)
	}
	return interrupt
}

func historyForwardChunkRoute(chunk *genx.MessageChunk) (historyForwardRoute, bool) {
	if chunk == nil || chunk.Ctrl == nil {
		return historyForwardRoute{}, false
	}
	label := strings.TrimSpace(chunk.Ctrl.Label)
	if label == "" {
		label = strings.TrimSpace(chunk.Name)
	}
	if label == "" {
		label = "assistant"
	}
	name := strings.TrimSpace(chunk.Name)
	if name == "" {
		name = label
	}
	role := chunk.Role
	if role == "" {
		role = genx.RoleModel
	}
	return historyForwardRoute{
		role:     role,
		name:     name,
		streamID: strings.TrimSpace(chunk.Ctrl.StreamID),
		label:    label,
	}, true
}

func historyForwardChunkMIME(chunk *genx.MessageChunk) (string, bool) {
	mimeType, ok := chunk.MIMEType()
	if !ok {
		return "", false
	}
	switch baseHistoryMIME(mimeType) {
	case "text/plain", "audio/opus":
		return mimeType, true
	default:
		return "", false
	}
}

func (r historyForwardRoute) key() historyForwardRouteKey {
	return historyForwardRouteKey{streamID: r.streamID, label: r.label}
}

func historyForwardInterruptedChunk(route historyForwardRoute, mimeType string) *genx.MessageChunk {
	if mimeType == "text/plain" {
		chunk := historyTextChunk(route.role, route.name, route.streamID, route.label, "", true)
		chunk.Ctrl.Error = historyReplayInterrupted
		return chunk
	}
	return &genx.MessageChunk{
		Role: route.role,
		Name: route.name,
		Part: &genx.Blob{MIMEType: mimeType},
		Ctrl: &genx.StreamCtrl{StreamID: route.streamID, Label: route.label, EndOfStream: true, Error: historyReplayInterrupted},
	}
}

func (o *historyOutput) startReplay(streamID string, role genx.Role, name string, label string, chunks []*genx.MessageChunk) error {
	if o == nil || o.output == nil {
		return fmt.Errorf("workspace history replay requires an active output stream")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var interrupt []*genx.MessageChunk
	o.replayMu.Lock()
	if o.replayCancel != nil {
		o.replayCancel()
	}
	if o.replayStreamID != "" {
		interrupt = historyReplayInterruptedChunks(o.replayRole, o.replayName, o.replayStreamID, o.replayLabel)
	}
	interrupt = append(interrupt, o.interruptForwardOutput()...)
	o.replaySeq++
	seq := o.replaySeq
	o.replayCancel = cancel
	o.replayStreamID = streamID
	o.replayRole = role
	o.replayName = name
	o.replayLabel = label
	o.replayMu.Unlock()
	if len(interrupt) > 0 {
		interruptedStreams := make(map[string]struct{}, len(interrupt))
		for _, chunk := range interrupt {
			if chunk != nil && chunk.Ctrl != nil {
				interruptedStreams[chunk.Ctrl.StreamID] = struct{}{}
			}
		}
		o.output.Discard(func(chunk *genx.MessageChunk) bool {
			if chunk == nil || chunk.Ctrl == nil {
				return false
			}
			_, interrupted := interruptedStreams[chunk.Ctrl.StreamID]
			return interrupted
		})
		if err := o.output.Add(interrupt...); err != nil {
			cancel()
			o.finishReplay(seq)
			return err
		}
	}
	go o.runReplay(ctx, seq, chunks)
	return nil
}

func (o *historyOutput) runReplay(ctx context.Context, seq uint64, chunks []*genx.MessageChunk) {
	defer o.finishReplay(seq)
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return
		}
		if !o.addReplayChunk(seq, chunk.Clone()) {
			return
		}
	}
}

func (o *historyOutput) addReplayChunk(seq uint64, chunk *genx.MessageChunk) bool {
	o.replayMu.Lock()
	defer o.replayMu.Unlock()
	if o.replaySeq != seq || o.replayStreamID == "" {
		return false
	}
	return o.output.Add(chunk) == nil
}

func (o *historyOutput) finishReplay(seq uint64) {
	if o == nil {
		return
	}
	o.replayMu.Lock()
	defer o.replayMu.Unlock()
	if o.replaySeq == seq {
		o.replayCancel = nil
	}
}

func (o *historyOutput) cancelReplay() {
	if o == nil {
		return
	}
	o.replayMu.Lock()
	cancel := o.replayCancel
	o.replayCancel = nil
	o.replayStreamID = ""
	o.replayRole = ""
	o.replayName = ""
	o.replayLabel = ""
	o.replaySeq++
	o.replayMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (o *historyOutput) notifyHistoryUpdated(lastUpdated time.Time) {
	if o == nil || o.output == nil {
		return
	}
	lastUpdated = lastUpdated.UTC()
	if lastUpdated.IsZero() {
		lastUpdated = time.Now().UTC()
	}
	o.notifyMu.Lock()
	if o.notifyLastUpdated.IsZero() || lastUpdated.After(o.notifyLastUpdated) {
		o.notifyLastUpdated = lastUpdated
	}
	if o.notifyTimer == nil {
		o.notifyTimer = time.AfterFunc(historyUpdatedDelay, o.flushHistoryUpdated)
	}
	o.notifyMu.Unlock()
}

func (o *historyOutput) flushHistoryUpdated() {
	if o == nil || o.output == nil {
		return
	}
	o.notifyMu.Lock()
	lastUpdated := o.notifyLastUpdated
	o.notifyLastUpdated = time.Time{}
	o.notifyTimer = nil
	o.notifyMu.Unlock()
	if lastUpdated.IsZero() {
		return
	}
	_ = o.output.Add(historyUpdatedChunk(lastUpdated))
}

func (o *historyOutput) cancelHistoryUpdated() {
	if o == nil {
		return
	}
	o.notifyMu.Lock()
	if o.notifyTimer != nil {
		o.notifyTimer.Stop()
		o.notifyTimer = nil
	}
	o.notifyLastUpdated = time.Time{}
	o.notifyMu.Unlock()
}

func historyUpdatedChunk(lastUpdated time.Time) *genx.MessageChunk {
	return &genx.MessageChunk{
		Ctrl: &genx.StreamCtrl{
			Label:     historyUpdatedLabel,
			Timestamp: lastUpdated.UTC().UnixMilli(),
		},
	}
}

func historyReplayInterruptedChunks(role genx.Role, name string, streamID string, label string) []*genx.MessageChunk {
	if role == "" {
		role = genx.RoleModel
	}
	if strings.TrimSpace(label) == "" {
		label = "assistant"
	}
	if strings.TrimSpace(name) == "" {
		name = label
	}
	textEOS := historyTextChunk(role, name, streamID, label, "", true)
	textEOS.Ctrl.Error = historyReplayInterrupted
	audioEOS := &genx.MessageChunk{
		Role: role,
		Name: name,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: true, Error: historyReplayInterrupted},
	}
	return []*genx.MessageChunk{textEOS, audioEOS}
}

type historyRecorder struct {
	history *workspace.HistoryStore
	gearID  string
	notify  func(time.Time)

	mu      sync.Mutex
	pending map[string]*historyPendingEntry
}

type historyPendingEntry struct {
	typ       string
	gearID    string
	name      string
	streamID  string
	label     string
	channels  map[string]bool
	audioMIME string
	text      strings.Builder
	audio     [][]byte
	oggAudio  bytes.Buffer
	pcmAudio  bytes.Buffer
	pcmWriter *codecconv.PCMToOggOpusEncoder
	pcmFormat pcm.Format
	createdAt time.Time
}

func newHistoryRecorder(history *workspace.HistoryStore, gearID string, notify func(time.Time)) *historyRecorder {
	return &historyRecorder{
		history: history,
		gearID:  strings.TrimSpace(gearID),
		notify:  notify,
		pending: make(map[string]*historyPendingEntry),
	}
}

func (r *historyRecorder) ObserveOutput(ctx context.Context, chunk *genx.MessageChunk) error {
	typ := historyEntryTypeAgent
	gearID := ""
	if chunk.Role == genx.RoleUser {
		if r == nil || strings.TrimSpace(r.gearID) == "" {
			return nil
		}
		typ = historyEntryTypeGear
		gearID = r.gearID
	}
	return r.observe(ctx, chunk, typ, gearID)
}

func (r *historyRecorder) Flush(ctx context.Context) error {
	return r.flushMatching(ctx, nil)
}

func (r *historyRecorder) discard() {
	if r == nil {
		return
	}
	r.mu.Lock()
	entries := make([]*historyPendingEntry, 0, len(r.pending))
	for key, entry := range r.pending {
		entries = append(entries, entry)
		delete(r.pending, key)
	}
	r.mu.Unlock()
	for _, entry := range entries {
		if entry != nil && entry.pcmWriter != nil {
			_ = entry.pcmWriter.Close()
		}
	}
}

func (r *historyRecorder) flushMatching(ctx context.Context, keep func(*historyPendingEntry) bool) error {
	r.mu.Lock()
	keys := make([]string, 0, len(r.pending))
	for key, entry := range r.pending {
		if keep != nil && keep(entry) {
			continue
		}
		keys = append(keys, key)
	}
	r.mu.Unlock()
	for _, key := range keys {
		if err := r.flush(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (r *historyRecorder) observe(ctx context.Context, chunk *genx.MessageChunk, typ string, gearID string) error {
	if r == nil || r.history == nil || chunk == nil {
		return nil
	}
	recordChunk := chunk
	var (
		entry    *historyPendingEntry
		mimeType string
	)
	switch part := chunk.Part.(type) {
	case genx.Text:
		mimeType, _ = chunk.MIMEType()
		if historyAgentRouteChannelEOS(chunk, part) {
			return r.completeRouteChannel(ctx, chunk, typ, mimeType)
		}
		entry = r.pendingEntry(recordChunk, typ, gearID)
		if err := entry.observeChannel(mimeType, chunk.IsEndOfStream()); err != nil {
			return err
		}
		if string(part) != "" {
			entry.text.WriteString(string(part))
		}
	case *genx.Blob:
		var ok bool
		mimeType, ok = chunk.MIMEType()
		if !ok {
			return fmt.Errorf("agenthost: history blob has missing or invalid MIME type")
		}
		if !isRecordableHistoryAudioMIME(mimeType) {
			break
		}
		if historyAgentRouteChannelEOS(chunk, part) {
			return r.completeRouteChannel(ctx, chunk, typ, mimeType)
		}
		if typ == historyEntryTypeGear {
			recordChunk = historyGearTranscriptChunk(chunk)
		}
		entry = r.pendingEntry(recordChunk, typ, gearID)
		if entry.audioMIME != "" && entry.audioMIME != mimeType {
			return fmt.Errorf("agenthost: history route changed audio MIME type from %q to %q", entry.audioMIME, mimeType)
		}
		entry.audioMIME = mimeType
		if err := entry.observeChannel(mimeType, chunk.IsEndOfStream()); err != nil {
			return err
		}
		if len(part.Data) == 0 {
			break
		}
		switch baseHistoryMIME(mimeType) {
		case "audio/opus":
			entry.audio = append(entry.audio, append([]byte(nil), part.Data...))
		case "audio/ogg", "application/ogg":
			_, _ = entry.oggAudio.Write(part.Data)
		default:
			format, ok := historyPCMFormat(mimeType)
			if !ok {
				break
			}
			if entry.pcmWriter == nil {
				writer, err := codecconv.NewPCMToOggOpusEncoder(&entry.pcmAudio, format.SampleRate(), format.Channels(), opus.ApplicationVoIP)
				if err != nil {
					return err
				}
				entry.pcmWriter = writer
				entry.pcmFormat = format
			} else if entry.pcmFormat != format {
				return fmt.Errorf("agenthost: history pcm stream changed format from %s to %s", entry.pcmFormat, format)
			}
			if _, err := entry.pcmWriter.Write(part.Data); err != nil {
				return err
			}
		}
	}
	if chunk.IsEndOfStream() {
		if chunk.Part == nil {
			return r.flushRoute(ctx, historyChunkStreamID(recordChunk))
		}
		if entry == nil || !entry.channelsComplete() {
			return nil
		}
		if mimeType != "text/plain" && deferGearAudioEntry(entry) {
			return nil
		}
		return r.flush(ctx, r.key(recordChunk, typ))
	}
	return nil
}

func historyAgentRouteChannelEOS(chunk *genx.MessageChunk, part any) bool {
	if chunk == nil || chunk.Role == genx.RoleUser || !chunk.IsEndOfStream() || chunk.Ctrl == nil {
		return false
	}
	name := strings.TrimSpace(chunk.Name)
	label := strings.TrimSpace(chunk.Ctrl.Label)
	if name == "" || label == "" || name != label {
		return false
	}
	switch value := part.(type) {
	case genx.Text:
		return value == ""
	case *genx.Blob:
		return value != nil && len(value.Data) == 0
	default:
		return false
	}
}

func (r *historyRecorder) completeRouteChannel(ctx context.Context, chunk *genx.MessageChunk, typ, mimeType string) error {
	streamID := historyChunkStreamID(chunk)
	label := strings.TrimSpace(chunk.Ctrl.Label)
	r.mu.Lock()
	keys := make([]string, 0, len(r.pending))
	for key, entry := range r.pending {
		if entry == nil || entry.typ != typ || entry.streamID != streamID || entry.label != label {
			continue
		}
		if _, observed := entry.channels[mimeType]; !observed {
			continue
		}
		entry.channels[mimeType] = true
		if entry.channelsComplete() {
			keys = append(keys, key)
		}
	}
	r.mu.Unlock()
	for _, key := range keys {
		if err := r.flush(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (e *historyPendingEntry) observeChannel(mimeType string, eos bool) error {
	if e.channels == nil {
		e.channels = make(map[string]bool)
	}
	if done, ok := e.channels[mimeType]; ok && done && !eos {
		return fmt.Errorf("agenthost: history MIME channel %q received data after EOS", mimeType)
	}
	e.channels[mimeType] = eos
	return nil
}

func (e *historyPendingEntry) channelsComplete() bool {
	if e == nil || len(e.channels) == 0 {
		return false
	}
	for _, done := range e.channels {
		if !done {
			return false
		}
	}
	return true
}

func historyOutputOnlyChunk(chunk *genx.MessageChunk) bool {
	return chunk != nil &&
		chunk.Role == genx.RoleUser &&
		chunk.Ctrl != nil &&
		strings.TrimSpace(chunk.Ctrl.Label) == genx.HistoryUserAudioLabel
}

func historyGearTranscriptChunk(chunk *genx.MessageChunk) *genx.MessageChunk {
	if chunk == nil {
		return nil
	}
	next := chunk.Clone()
	next.Name = "transcript"
	if next.Ctrl == nil {
		next.Ctrl = &genx.StreamCtrl{}
	}
	next.Ctrl.Label = "transcript"
	return next
}

func (r *historyRecorder) pendingEntry(chunk *genx.MessageChunk, typ string, gearID string) *historyPendingEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := r.key(chunk, typ)
	entry := r.pending[key]
	if entry == nil {
		entry = &historyPendingEntry{
			typ:       typ,
			gearID:    strings.TrimSpace(gearID),
			name:      historyChunkName(chunk, typ),
			streamID:  historyChunkStreamID(chunk),
			label:     historyChunkLabel(chunk),
			createdAt: time.Now().UTC(),
		}
		r.pending[key] = entry
	}
	return entry
}

func deferGearAudioEntry(entry *historyPendingEntry) bool {
	return entry != nil && entry.typ == historyEntryTypeGear && strings.TrimSpace(entry.text.String()) == "" && (len(entry.audio) > 0 || entry.oggAudio.Len() > 0 || entry.pcmWriter != nil)
}

func (r *historyRecorder) flushRoute(ctx context.Context, streamID string) error {
	return r.flushMatching(ctx, func(entry *historyPendingEntry) bool {
		return entry == nil || entry.streamID != streamID
	})
}

func (r *historyRecorder) flush(ctx context.Context, key string) error {
	r.mu.Lock()
	entry := r.pending[key]
	delete(r.pending, key)
	r.mu.Unlock()
	if entry == nil {
		return nil
	}
	if entry.oggAudio.Len() > 0 {
		frames, err := historyOpusFramesFromOgg(entry.oggAudio.Bytes())
		if err != nil {
			return err
		}
		entry.audio = append(entry.audio, frames...)
	}
	var pcmAsset []byte
	if entry.pcmWriter != nil {
		if err := entry.pcmWriter.Close(); err != nil {
			return err
		}
		pcmAsset = append([]byte(nil), entry.pcmAudio.Bytes()...)
	}
	text := entry.text.String()
	if strings.TrimSpace(text) == "" && len(entry.audio) == 0 && len(pcmAsset) == 0 {
		return nil
	}
	req := workspace.AppendHistoryRequest{
		Type:      entry.typ,
		GearID:    entry.gearID,
		Name:      entry.name,
		Text:      text,
		CreatedAt: entry.createdAt,
	}
	if len(entry.audio) > 0 {
		audio, err := historyOggOpusAsset(entry.audio)
		if err != nil {
			return err
		}
		req.Asset = &workspace.AppendHistoryAsset{
			MIMEType: "audio/ogg; codecs=opus",
			Data:     audio,
		}
	} else if len(pcmAsset) > 0 {
		req.Asset = &workspace.AppendHistoryAsset{
			MIMEType: "audio/ogg; codecs=opus",
			Data:     pcmAsset,
		}
	}
	stored, err := r.history.Append(ctx, req)
	if err != nil {
		return err
	}
	if r.notify != nil {
		r.notify(stored.CreatedAt)
	}
	return nil
}

func historyAudioReplayChunks(role genx.Role, name, streamID, label, mimeType string, data []byte) ([]*genx.MessageChunk, error) {
	mimeType = strings.TrimSpace(mimeType)
	if len(data) == 0 {
		return nil, nil
	}
	var frames [][]byte
	switch baseHistoryMIME(mimeType) {
	case "audio/ogg", "application/ogg":
		var err error
		frames, err = historyOpusFramesFromOgg(data)
		if err != nil {
			return nil, err
		}
	case "audio/opus":
		frames = [][]byte{append([]byte(nil), data...)}
	default:
		return []*genx.MessageChunk{
			{Role: role, Name: name, Part: &genx.Blob{MIMEType: mimeType, Data: data}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label}},
			{Role: role, Name: name, Part: &genx.Blob{MIMEType: mimeType}, Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: true}},
		}, nil
	}
	chunks := make([]*genx.MessageChunk, 0, len(frames)+2)
	chunks = append(chunks, &genx.MessageChunk{
		Role: role,
		Name: name,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, BeginOfStream: true},
	})
	for _, frame := range frames {
		if len(frame) == 0 {
			continue
		}
		chunks = append(chunks, &genx.MessageChunk{
			Role: role,
			Name: name,
			Part: &genx.Blob{MIMEType: "audio/opus", Data: frame},
			Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label},
		})
	}
	chunks = append(chunks, &genx.MessageChunk{
		Role: role,
		Name: name,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: true},
	})
	return chunks, nil
}

func historyOggOpusAsset(frames [][]byte) ([]byte, error) {
	var out bytes.Buffer
	err := codecconv.OpusPacketsToOgg(&out, historyOggOpusSampleRate, historyOggOpusChannels, frames)
	if err != nil {
		return nil, fmt.Errorf("agenthost: write history ogg opus: %w", err)
	}
	return out.Bytes(), nil
}

func historyOpusFramesFromOgg(data []byte) ([][]byte, error) {
	var frames [][]byte
	for packet, err := range codecconv.OggOpusPackets(bytes.NewReader(data)) {
		if err != nil {
			return nil, fmt.Errorf("agenthost: read history ogg opus: %w", err)
		}
		frames = append(frames, packet)
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("agenthost: history ogg opus has no audio packets")
	}
	return frames, nil
}

func baseHistoryMIME(mimeType string) string {
	if idx := strings.IndexByte(mimeType, ';'); idx >= 0 {
		mimeType = mimeType[:idx]
	}
	return strings.ToLower(strings.TrimSpace(mimeType))
}

func isHistoryAudioMIME(mimeType string) bool {
	mimeType = baseHistoryMIME(mimeType)
	return strings.HasPrefix(mimeType, "audio/") || mimeType == "application/ogg"
}

func isRecordableHistoryAudioMIME(mimeType string) bool {
	switch baseHistoryMIME(mimeType) {
	case "audio/opus", "audio/ogg", "application/ogg":
		return true
	default:
		_, ok := historyPCMFormat(mimeType)
		return ok
	}
}

func historyPCMFormat(mimeType string) (pcm.Format, bool) {
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(mimeType))
	if err != nil {
		mediaType = baseHistoryMIME(mimeType)
		params = nil
	}
	switch strings.ToLower(mediaType) {
	case "audio/pcm", "audio/x-pcm":
		return pcm.L16Mono16K, true
	case "audio/l16":
		channels := 1
		if raw := strings.TrimSpace(params["channels"]); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n != 1 {
				return 0, false
			}
			channels = n
		}
		if channels != 1 {
			return 0, false
		}
		switch strings.TrimSpace(params["rate"]) {
		case "16000", "":
			return pcm.L16Mono16K, true
		case "24000":
			return pcm.L16Mono24K, true
		case "48000":
			return pcm.L16Mono48K, true
		default:
			return 0, false
		}
	default:
		return 0, false
	}
}

func (r *historyRecorder) key(chunk *genx.MessageChunk, typ string) string {
	return typ + ":" + historyChunkStreamID(chunk) + ":" + historyChunkLabel(chunk) + ":" + historyChunkName(chunk, typ)
}

func historyChunkLabel(chunk *genx.MessageChunk) string {
	if chunk != nil && chunk.Ctrl != nil {
		return strings.TrimSpace(chunk.Ctrl.Label)
	}
	return ""
}

func historyChunkStreamID(chunk *genx.MessageChunk) string {
	if chunk != nil && chunk.Ctrl != nil && chunk.Ctrl.StreamID != "" {
		return chunk.Ctrl.StreamID
	}
	return "default"
}

func historyChunkName(chunk *genx.MessageChunk, typ string) string {
	if chunk != nil {
		if strings.TrimSpace(chunk.Name) != "" {
			return strings.TrimSpace(chunk.Name)
		}
		if chunk.Ctrl != nil && strings.TrimSpace(chunk.Ctrl.Label) != "" {
			return strings.TrimSpace(chunk.Ctrl.Label)
		}
	}
	if typ == historyEntryTypeGear {
		return "gear"
	}
	return "agent"
}

func historyTextChunk(role genx.Role, name, streamID, label, text string, eos bool) *genx.MessageChunk {
	return &genx.MessageChunk{
		Role: role,
		Name: name,
		Part: genx.Text(text),
		Ctrl: &genx.StreamCtrl{StreamID: streamID, Label: label, EndOfStream: eos},
	}
}

func historyIsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
