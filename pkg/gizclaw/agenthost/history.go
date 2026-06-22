package agenthost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/audio/codec/opus"
	"github.com/GizClaw/gizclaw-go/pkg/audio/codecconv"
	"github.com/GizClaw/gizclaw-go/pkg/audio/pcm"
	"github.com/GizClaw/gizclaw-go/pkg/genx"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/workspace"
)

const (
	historyEntryTypeGear  = "gear"
	historyEntryTypeAgent = "agent"

	historyOggOpusSampleRate = 48000
	historyOggOpusChannels   = 1
	historyReplayFrameDelay  = 20 * time.Millisecond
	historyReplayInterrupted = "interrupted"
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
	output   *genx.StreamBuilder
	streamID uint64

	replayMu       sync.Mutex
	replayCancel   context.CancelFunc
	replaySeq      uint64
	replayStreamID string
	replayRole     genx.Role
	replayName     string
	replayLabel    string

	forwardMu          sync.Mutex
	activeForward      map[historyForwardRouteKey]historyForwardRoute
	interruptedForward map[historyForwardChunkKey]struct{}
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
	kind string
}

func (a *historyAgent) Transform(ctx context.Context, pattern string, input genx.Stream) (genx.Stream, error) {
	if a == nil || a.Agent == nil {
		return nil, fmt.Errorf("agenthost: history agent is nil")
	}
	recorder := newHistoryRecorder(a.history, historyGearID(ctx))
	wrappedInput := &historyInputStream{Stream: input, ctx: ctx, recorder: recorder}
	agentOutput, err := a.Agent.Transform(ctx, pattern, wrappedInput)
	if err != nil {
		return nil, err
	}
	output := genx.NewStreamBuilder((&genx.ModelContextBuilder{}).Build(), 256)
	a.outputMu.Lock()
	a.output = output
	a.streamID++
	epoch := a.streamID
	a.outputMu.Unlock()
	go a.forwardOutput(ctx, epoch, agentOutput, output, recorder)
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
	output, streamID, ok := a.currentOutput()
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
	if err := a.startReplay(output, streamID, role, name, label, chunks); err != nil {
		message := err.Error()
		return apitypes.PeerRunHistoryPlayResponse{Accepted: false, HistoryId: req.HistoryId, State: "unavailable", Message: &message}, nil
	}
	return apitypes.PeerRunHistoryPlayResponse{Accepted: true, HistoryId: req.HistoryId, State: "played"}, nil
}

func (a *historyAgent) forwardOutput(ctx context.Context, epoch uint64, input genx.Stream, output *genx.StreamBuilder, recorder *historyRecorder) {
	defer input.Close()
	for {
		if err := ctx.Err(); err != nil {
			_ = output.Abort(err)
			a.clearOutput(epoch)
			return
		}
		chunk, err := input.Next()
		if err != nil {
			if flushErr := recorder.Flush(ctx); flushErr != nil {
				a.clearOutput(epoch)
				_ = output.Abort(flushErr)
				return
			}
			a.clearOutput(epoch)
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
		if a.observeForwardChunk(chunk) {
			continue
		}
		if err := recorder.ObserveOutput(ctx, chunk); err != nil {
			a.clearOutput(epoch)
			_ = output.Abort(err)
			return
		}
		if err := output.Add(chunk.Clone()); err != nil {
			_ = recorder.Flush(ctx)
			a.clearOutput(epoch)
			return
		}
	}
}

func (a *historyAgent) currentOutput() (*genx.StreamBuilder, string, bool) {
	a.outputMu.Lock()
	defer a.outputMu.Unlock()
	if a.output == nil {
		return nil, "", false
	}
	return a.output, fmt.Sprintf("history-replay-%d", time.Now().UnixNano()), true
}

func (a *historyAgent) clearOutput(epoch uint64) {
	clearReplay := false
	a.outputMu.Lock()
	if a.streamID == epoch {
		a.output = nil
		clearReplay = true
	}
	a.outputMu.Unlock()
	if clearReplay {
		a.cancelReplay()
		a.clearForwardOutput()
	}
}

func (a *historyAgent) clearForwardOutput() {
	a.forwardMu.Lock()
	defer a.forwardMu.Unlock()
	a.activeForward = nil
	a.interruptedForward = nil
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

func (a *historyAgent) observeForwardChunk(chunk *genx.MessageChunk) bool {
	route, kind, ok := historyForwardChunkRoute(chunk)
	if !ok {
		return false
	}
	key := historyForwardChunkKey{historyForwardRouteKey: route.key(), kind: kind}
	a.forwardMu.Lock()
	defer a.forwardMu.Unlock()
	if _, interrupted := a.interruptedForward[key]; interrupted {
		if chunk.IsEndOfStream() {
			delete(a.interruptedForward, key)
		}
		return true
	}
	if chunk.IsEndOfStream() {
		delete(a.activeForward, route.key())
		return false
	}
	if a.activeForward == nil {
		a.activeForward = make(map[historyForwardRouteKey]historyForwardRoute)
	}
	a.activeForward[route.key()] = route
	return false
}

func (a *historyAgent) interruptForwardOutput() []*genx.MessageChunk {
	a.forwardMu.Lock()
	defer a.forwardMu.Unlock()
	if len(a.activeForward) == 0 {
		return nil
	}
	interrupt := make([]*genx.MessageChunk, 0, len(a.activeForward)*2)
	if a.interruptedForward == nil {
		a.interruptedForward = make(map[historyForwardChunkKey]struct{})
	}
	for key, route := range a.activeForward {
		interrupt = append(interrupt, historyForwardInterruptedChunks(route)...)
		a.interruptedForward[historyForwardChunkKey{historyForwardRouteKey: key, kind: "text"}] = struct{}{}
		a.interruptedForward[historyForwardChunkKey{historyForwardRouteKey: key, kind: "audio"}] = struct{}{}
		delete(a.activeForward, key)
	}
	return interrupt
}

func historyForwardChunkRoute(chunk *genx.MessageChunk) (historyForwardRoute, string, bool) {
	if chunk == nil || chunk.Ctrl == nil {
		return historyForwardRoute{}, "", false
	}
	kind := historyForwardChunkKind(chunk)
	if kind == "" {
		return historyForwardRoute{}, "", false
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
	}, kind, true
}

func historyForwardChunkKind(chunk *genx.MessageChunk) string {
	switch part := chunk.Part.(type) {
	case genx.Text:
		return "text"
	case *genx.Blob:
		if baseHistoryMIME(part.MIMEType) == "audio/opus" {
			return "audio"
		}
	}
	return ""
}

func (r historyForwardRoute) key() historyForwardRouteKey {
	return historyForwardRouteKey{streamID: r.streamID, label: r.label}
}

func historyForwardInterruptedChunks(route historyForwardRoute) []*genx.MessageChunk {
	textEOS := historyTextChunk(route.role, route.name, route.streamID, route.label, "", true)
	textEOS.Ctrl.Error = historyReplayInterrupted
	audioEOS := &genx.MessageChunk{
		Role: route.role,
		Name: route.name,
		Part: &genx.Blob{MIMEType: "audio/opus"},
		Ctrl: &genx.StreamCtrl{StreamID: route.streamID, Label: route.label, EndOfStream: true, Error: historyReplayInterrupted},
	}
	return []*genx.MessageChunk{textEOS, audioEOS}
}

func (a *historyAgent) startReplay(output *genx.StreamBuilder, streamID string, role genx.Role, name string, label string, chunks []*genx.MessageChunk) error {
	if output == nil {
		return fmt.Errorf("workspace history replay requires an active output stream")
	}
	ctx, cancel := context.WithCancel(context.Background())
	var interrupt []*genx.MessageChunk
	a.replayMu.Lock()
	if a.replayCancel != nil {
		a.replayCancel()
	}
	if a.replayStreamID != "" {
		interrupt = historyReplayInterruptedChunks(a.replayRole, a.replayName, a.replayStreamID, a.replayLabel)
	}
	interrupt = append(interrupt, a.interruptForwardOutput()...)
	a.replaySeq++
	seq := a.replaySeq
	a.replayCancel = cancel
	a.replayStreamID = streamID
	a.replayRole = role
	a.replayName = name
	a.replayLabel = label
	a.replayMu.Unlock()
	if len(interrupt) > 0 {
		if err := output.Add(interrupt...); err != nil {
			cancel()
			a.finishReplay(seq)
			return err
		}
	}
	go a.runReplay(ctx, seq, output, chunks)
	return nil
}

func (a *historyAgent) runReplay(ctx context.Context, seq uint64, output *genx.StreamBuilder, chunks []*genx.MessageChunk) {
	defer a.finishReplay(seq)
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return
		}
		if !a.isCurrentReplay(seq) {
			return
		}
		if err := output.Add(chunk.Clone()); err != nil {
			return
		}
		if historyReplayNeedsPace(chunk) {
			if err := a.waitReplayFrame(ctx, seq); err != nil {
				return
			}
		}
	}
}

func (a *historyAgent) waitReplayFrame(ctx context.Context, seq uint64) error {
	if !a.isCurrentReplay(seq) {
		return context.Canceled
	}
	timer := time.NewTimer(historyReplayFrameDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	if !a.isCurrentReplay(seq) {
		return context.Canceled
	}
	return nil
}

func (a *historyAgent) finishReplay(seq uint64) {
	a.replayMu.Lock()
	defer a.replayMu.Unlock()
	if a.replaySeq == seq {
		a.replayCancel = nil
		a.replayStreamID = ""
		a.replayRole = ""
		a.replayName = ""
		a.replayLabel = ""
	}
}

func (a *historyAgent) cancelReplay() {
	a.replayMu.Lock()
	cancel := a.replayCancel
	a.replayCancel = nil
	a.replayStreamID = ""
	a.replayRole = ""
	a.replayName = ""
	a.replayLabel = ""
	a.replaySeq++
	a.replayMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *historyAgent) isCurrentReplay(seq uint64) bool {
	a.replayMu.Lock()
	defer a.replayMu.Unlock()
	return a.replaySeq == seq && a.replayCancel != nil
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

func historyReplayNeedsPace(chunk *genx.MessageChunk) bool {
	if chunk == nil || chunk.IsEndOfStream() {
		return false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	return ok && len(blob.Data) > 0 && baseHistoryMIME(blob.MIMEType) == "audio/opus"
}

type historyInputStream struct {
	genx.Stream
	ctx      context.Context
	recorder *historyRecorder
}

func (s *historyInputStream) Next() (*genx.MessageChunk, error) {
	chunk, err := s.Stream.Next()
	if err == nil && chunk != nil {
		if recordErr := s.recorder.ObserveInput(s.ctx, chunk); recordErr != nil {
			return nil, recordErr
		}
	}
	if err != nil {
		if flushErr := s.recorder.FlushReady(s.ctx); flushErr != nil {
			return nil, flushErr
		}
	}
	return chunk, err
}

type historyRecorder struct {
	history *workspace.HistoryStore
	gearID  string

	mu      sync.Mutex
	pending map[string]*historyPendingEntry
}

type historyPendingEntry struct {
	typ       string
	gearID    string
	name      string
	text      strings.Builder
	audio     [][]byte
	oggAudio  bytes.Buffer
	pcmAudio  bytes.Buffer
	pcmWriter *codecconv.PCMToOggOpusEncoder
	pcmFormat pcm.Format
	createdAt time.Time
}

func newHistoryRecorder(history *workspace.HistoryStore, gearID string) *historyRecorder {
	return &historyRecorder{
		history: history,
		gearID:  strings.TrimSpace(gearID),
		pending: make(map[string]*historyPendingEntry),
	}
}

func (r *historyRecorder) ObserveInput(ctx context.Context, chunk *genx.MessageChunk) error {
	if r == nil || strings.TrimSpace(r.gearID) == "" {
		return nil
	}
	return r.observe(ctx, chunk, historyEntryTypeGear, r.gearID)
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

func (r *historyRecorder) FlushReady(ctx context.Context) error {
	return r.flushMatching(ctx, func(entry *historyPendingEntry) bool {
		return deferGearAudioEntry(entry)
	})
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
	switch part := chunk.Part.(type) {
	case genx.Text:
		entry := r.pendingEntry(recordChunk, typ, gearID)
		if string(part) != "" {
			entry.text.WriteString(string(part))
		}
	case *genx.Blob:
		if part == nil || !isHistoryAudioMIME(part.MIMEType) {
			break
		}
		if typ == historyEntryTypeGear {
			recordChunk = historyGearTranscriptChunk(chunk)
		}
		entry := r.pendingEntry(recordChunk, typ, gearID)
		if len(part.Data) == 0 {
			break
		}
		switch baseHistoryMIME(part.MIMEType) {
		case "audio/opus":
			entry.audio = append(entry.audio, append([]byte(nil), part.Data...))
		case "audio/ogg", "application/ogg":
			_, _ = entry.oggAudio.Write(part.Data)
		default:
			format, ok := historyPCMFormat(part.MIMEType)
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
		if r.deferGearAudioFlush(recordChunk, typ) {
			return nil
		}
		if err := r.flush(ctx, r.key(recordChunk, typ)); err != nil {
			return err
		}
	}
	return nil
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
			createdAt: time.Now().UTC(),
		}
		r.pending[key] = entry
	}
	return entry
}

func (r *historyRecorder) deferGearAudioFlush(chunk *genx.MessageChunk, typ string) bool {
	if typ != historyEntryTypeGear || chunk == nil {
		return false
	}
	if _, ok := chunk.Part.(genx.Text); ok {
		return false
	}
	blob, ok := chunk.Part.(*genx.Blob)
	if !ok || !isHistoryAudioMIME(blob.MIMEType) {
		return false
	}
	r.mu.Lock()
	entry := r.pending[r.key(chunk, typ)]
	defer r.mu.Unlock()
	return deferGearAudioEntry(entry)
}

func deferGearAudioEntry(entry *historyPendingEntry) bool {
	return entry != nil && entry.typ == historyEntryTypeGear && strings.TrimSpace(entry.text.String()) == "" && (len(entry.audio) > 0 || entry.oggAudio.Len() > 0 || entry.pcmWriter != nil)
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
	_, err := r.history.Append(ctx, req)
	return err
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
	chunks := make([]*genx.MessageChunk, 0, len(frames)+1)
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
	streamID := ""
	label := ""
	if chunk != nil && chunk.Ctrl != nil {
		streamID = chunk.Ctrl.StreamID
		label = chunk.Ctrl.Label
	}
	if streamID == "" {
		streamID = "default"
	}
	return typ + ":" + streamID + ":" + label + ":" + historyChunkName(chunk, typ)
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
