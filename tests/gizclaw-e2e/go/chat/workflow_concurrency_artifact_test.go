//go:build gizclaw_e2e

package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const workflowConcurrencyArtifactVersion = 1

type workflowConcurrencyArtifact struct {
	Version     int                                `json:"version"`
	CreatedAt   time.Time                          `json:"created_at"`
	Repository  workflowConcurrencyRepository      `json:"repository"`
	Workflow    workflowConcurrencyWorkflow        `json:"workflow"`
	Scenario    workflowConcurrencyScenario        `json:"scenario"`
	Concurrency int                                `json:"concurrency"`
	Barrier     workflowConcurrencyBarrierArtifact `json:"barrier"`
	Driver      workflowConcurrencyDriverArtifact  `json:"driver"`
	Lanes       []workflowConcurrencyLaneResult    `json:"lanes"`
}

type workflowConcurrencyRepository struct {
	Head  string `json:"head"`
	Dirty bool   `json:"dirty"`
}

type workflowConcurrencyWorkflow struct {
	Family                string `json:"family"`
	Fixture               string `json:"fixture"`
	InputMode             string `json:"input_mode"`
	RealtimeInputKeptOpen bool   `json:"realtime_input_kept_open"`
	RealtimeSilenceFed    bool   `json:"realtime_silence_fed"`
	RealtimeTailSilenceMS int64  `json:"realtime_tail_silence_ms,omitempty"`
}

type workflowConcurrencyBarrierArtifact struct {
	Ready      int       `json:"ready"`
	ReadyAt    time.Time `json:"ready_at"`
	ReleasedAt time.Time `json:"released_at"`
}

type workflowConcurrencyDriverArtifact struct {
	Before workflowConcurrencyRuntimeSnapshot `json:"before"`
	After  workflowConcurrencyRuntimeSnapshot `json:"after"`
}

type workflowConcurrencyRuntimeSnapshot struct {
	CapturedAt  time.Time `json:"captured_at"`
	Goroutines  int       `json:"goroutines"`
	HeapAlloc   uint64    `json:"heap_alloc"`
	HeapObjects uint64    `json:"heap_objects"`
}

type workflowConcurrencyLaneResult struct {
	Index         int                              `json:"index"`
	PublicKey     string                           `json:"public_key"`
	Workspace     string                           `json:"workspace"`
	ReadyAt       time.Time                        `json:"ready_at"`
	ReleasedAt    time.Time                        `json:"released_at"`
	FinishedAt    time.Time                        `json:"finished_at"`
	StartedAt     time.Time                        `json:"runtime_started_at"`
	RuntimeChecks []time.Time                      `json:"runtime_started_at_checks,omitempty"`
	Turns         []workflowConcurrencyTurnResult  `json:"turns,omitempty"`
	Cleanup       workflowConcurrencyCleanupResult `json:"cleanup"`
	Error         string                           `json:"error,omitempty"`
}

type workflowConcurrencyTurnResult struct {
	Index                int       `json:"index"`
	InputStreamID        string    `json:"input_stream_id"`
	AssistantStreamID    string    `json:"assistant_stream_id,omitempty"`
	TranscriptStreamID   string    `json:"transcript_stream_id,omitempty"`
	StartedAt            time.Time `json:"started_at"`
	InputDoneAt          time.Time `json:"input_done_at"`
	InputEOSSent         bool      `json:"input_eos_sent"`
	AudioEpochAt         time.Time `json:"audio_epoch_at,omitempty"`
	InterruptedAt        time.Time `json:"interrupted_at,omitempty"`
	CompletedAt          time.Time `json:"completed_at,omitempty"`
	AssistantTextChars   int       `json:"assistant_text_chars"`
	TranscriptChars      int       `json:"transcript_chars"`
	AssistantBOS         int       `json:"assistant_bos"`
	EventCount           int       `json:"event_count"`
	AudioPackets         int       `json:"audio_packets"`
	AudioBytes           int       `json:"audio_bytes"`
	InterruptedTerminals int       `json:"interrupted_terminals"`
	InterruptedText      int       `json:"interrupted_text_terminals"`
	InterruptedAudio     int       `json:"interrupted_audio_terminals"`
	TranscriptDone       bool      `json:"transcript_done"`
	AssistantTextDone    bool      `json:"assistant_text_done"`
	AssistantAudioDone   bool      `json:"assistant_audio_done"`
}

type workflowConcurrencyCleanupResult struct {
	StreamClosed     bool     `json:"stream_closed"`
	RuntimeStopped   bool     `json:"runtime_stopped"`
	WorkspaceDeleted bool     `json:"workspace_deleted"`
	ClientClosed     bool     `json:"client_closed"`
	ServeJoined      bool     `json:"serve_joined"`
	PeerOffline      bool     `json:"peer_offline"`
	Errors           []string `json:"errors,omitempty"`
}

func newWorkflowConcurrencyArtifact(
	spec workflowConcurrencySpec,
	scenario workflowConcurrencyScenario,
	concurrency int,
) *workflowConcurrencyArtifact {
	head, dirty := workflowConcurrencyRepositoryState()
	var realtimeTailSilenceMS int64
	if spec.FeedRealtimeSilence {
		realtimeTailSilenceMS = workflowConcurrencyRealtimeTailSilence(spec).Milliseconds()
	}
	return &workflowConcurrencyArtifact{
		Version:    workflowConcurrencyArtifactVersion,
		CreatedAt:  time.Now(),
		Repository: workflowConcurrencyRepository{Head: head, Dirty: dirty},
		Workflow: workflowConcurrencyWorkflow{
			Family: spec.Name, Fixture: spec.Fixture, InputMode: spec.InputMode,
			RealtimeInputKeptOpen: spec.KeepRealtimeInputOpen,
			RealtimeSilenceFed:    spec.FeedRealtimeSilence,
			RealtimeTailSilenceMS: realtimeTailSilenceMS,
		},
		Scenario:    scenario,
		Concurrency: concurrency,
	}
}

func (a *workflowConcurrencyArtifact) captureBefore() {
	a.Driver.Before = captureWorkflowConcurrencyRuntime()
}

func (a *workflowConcurrencyArtifact) captureAfter() {
	a.Driver.After = captureWorkflowConcurrencyRuntime()
}

func (a *workflowConcurrencyArtifact) captureLanes(lanes []*workflowConcurrencyLane) {
	a.Lanes = make([]workflowConcurrencyLaneResult, 0, len(lanes))
	for _, lane := range lanes {
		if lane == nil {
			continue
		}
		lane.Result.Error = redactWorkflowConcurrencyText(lane.Result.Error)
		for index := range lane.Result.Cleanup.Errors {
			lane.Result.Cleanup.Errors[index] = redactWorkflowConcurrencyText(lane.Result.Cleanup.Errors[index])
		}
		a.Lanes = append(a.Lanes, lane.Result)
	}
}

func workflowConcurrencyRepositoryState() (string, bool) {
	headOutput, headErr := exec.Command("git", "rev-parse", "HEAD").Output()
	statusOutput, statusErr := exec.Command("git", "status", "--porcelain").Output()
	head := strings.TrimSpace(string(headOutput))
	if headErr != nil {
		head = "unknown"
	}
	return head, statusErr != nil || len(strings.TrimSpace(string(statusOutput))) > 0
}

func writeWorkflowConcurrencyArtifact(artifact *workflowConcurrencyArtifact) error {
	if artifact == nil {
		return fmt.Errorf("artifact is nil")
	}
	dir := strings.TrimSpace(os.Getenv("GIZCLAW_E2E_WORKFLOW_CONCURRENCY_ARTIFACT_DIR"))
	if dir == "" {
		dir = filepath.Join("..", "..", "testdata", "workflow-concurrency")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create artifact directory: %w", err)
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("encode artifact: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(dir, ".workflow-concurrency-*.json")
	if err != nil {
		return fmt.Errorf("create artifact temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write artifact: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync artifact: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close artifact: %w", err)
	}
	name := fmt.Sprintf("%s-%s-%s.json", artifact.CreatedAt.UTC().Format("20060102T150405.000000000Z"), artifact.Workflow.Family, artifact.Scenario)
	finalPath := filepath.Join(dir, name)
	if err := os.Rename(temporaryName, finalPath); err != nil {
		return fmt.Errorf("publish artifact: %w", err)
	}
	removeTemporary = false
	fmt.Printf("workflow_concurrency_artifact=%s\n", finalPath)
	return nil
}

var workflowConcurrencySecretPattern = regexp.MustCompile(`(?i)(authorization|api[_-]?key|credential|private[_-]?key|registration[_-]?token|bearer)(["' :=]+)[^\s,"']+`)

func redactWorkflowConcurrencyText(value string) string {
	return workflowConcurrencySecretPattern.ReplaceAllString(value, "$1$2[redacted]")
}

func workflowConcurrencyErrorText(err error) string {
	if err == nil {
		return ""
	}
	return redactWorkflowConcurrencyText(err.Error())
}
