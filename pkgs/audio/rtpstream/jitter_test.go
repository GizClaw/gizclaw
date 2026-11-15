package rtpstream

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/packet"
	"github.com/pion/rtp"
)

func testInfo(source string, seq uint16, timestamp, duration uint32) *packet.OpusInfo {
	return &packet.OpusInfo{SourceID: source, Header: rtp.Header{Version: 2, SSRC: 7, SequenceNumber: seq, Timestamp: timestamp}, Duration48: duration}
}

func TestInitialReorderAndExactLossDuration(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	if got := b.Push([]byte{2}, testInfo("a", 11, 1960, 960), now); len(got) != 0 {
		t.Fatal("initial packet escaped the reorder window")
	}
	b.Push([]byte{1}, testInfo("a", 10, 1000, 960), now.Add(time.Millisecond))
	got := b.Pop(now.Add(40 * time.Millisecond))
	if len(got) != 2 || got[0].Info.Header.SequenceNumber != 10 || got[1].Info.Header.SequenceNumber != 11 {
		t.Fatalf("initial order = %+v", got)
	}
	b.Push([]byte{4}, testInfo("a", 14, 4840, 960), now.Add(50*time.Millisecond))
	if got := b.Pop(now.Add(89 * time.Millisecond)); len(got) != 0 {
		t.Fatal("gap released before its deadline")
	}
	got = b.Pop(now.Add(90 * time.Millisecond))
	if len(got) != 2 || !got[0].Info.Missing || got[0].Info.Duration48 != 1920 || got[0].Info.Header.Timestamp != 2920 || len(got[0].Payload) != 0 {
		t.Fatalf("gap = %+v", got)
	}
	if got[1].Info.Header.SequenceNumber != 14 || got[1].Info.Discontinuity {
		t.Fatal("real packet was replaced or unnecessarily reset")
	}
	if got := b.Push([]byte{3}, testInfo("a", 12, 2920, 960), now.Add(100*time.Millisecond)); len(got) != 0 {
		t.Fatal("late packet escaped its delivered sequence fence")
	}
}

func TestSequenceTimestampWrapAndDTX(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 65535, ^uint32(0)-959, 960), now)
	b.Push([]byte{2}, testInfo("a", 0, 0, 480), now)
	got := b.Flush()
	if len(got) != 2 || got[0].Info.Header.SequenceNumber != 65535 || got[1].Info.Header.SequenceNumber != 0 {
		t.Fatal("sequence wrap was not preserved")
	}
	got = b.Push([]byte{3}, testInfo("a", 1, 48000, 960), now)
	if len(got) != 1 || got[0].Info.Missing {
		t.Fatal("a timestamp jump without a sequence gap invented PLC")
	}
}

func TestMarkedTalkspurtDoesNotConcealDTX(t *testing.T) {
	for _, test := range []struct {
		name      string
		sequence  uint16
		timestamp uint32
		marked    bool
	}{
		{"marked talkspurt", 10, 0, true},
		{"marked across wrap", 65534, ^uint32(0) - 959, true},
		{"unmarked loss control", 10, 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(100, 0)
			b := New(Config{})
			b.Push([]byte{1}, testInfo("a", test.sequence, test.timestamp, 960), now)
			requireRealSequences(t, b.Flush(), test.sequence)
			// One lost 20 ms packet plus 40 ms of DTX fits the legal loss
			// duration bound, but a marked talkspurt cannot prove 60 ms of loss.
			next := testInfo("a", test.sequence+2, test.timestamp+3840, 960)
			next.Header.Marker = test.marked
			b.Push([]byte{2}, next, now)
			got := b.Flush()
			if test.marked {
				requireRealSequences(t, got, next.Header.SequenceNumber)
				if !got[0].Info.Discontinuity || !got[0].Info.Header.Marker {
					t.Fatal("marked talkspurt lost its discontinuity or RTP marker")
				}
			} else {
				if len(got) != 2 || !got[0].Info.Missing || got[0].Info.Duration48 != 2880 {
					t.Fatalf("unmarked loss no longer produces bounded concealment: %+v", got)
				}
				requireRealSequences(t, got[1:], next.Header.SequenceNumber)
				if got[1].Info.Discontinuity {
					t.Fatal("ordinary loss unnecessarily reset decoder history")
				}
			}
			if got := b.Push([]byte{3}, testInfo("a", test.sequence+1, test.timestamp+960, 960), now); len(got) != 0 {
				t.Fatal("late pre-talkspurt packet crossed the sequence fence")
			}
			requireRealSequences(t, b.Push([]byte{4}, testInfo("a", test.sequence+3, next.Header.Timestamp+960, 960), now), test.sequence+3)
		})
	}
}

func TestLargeGapResetsWithoutUnboundedPLC(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1, 0, 960), now)
	b.Flush()
	b.Push([]byte{2}, testInfo("a", 3, 48000*60, 960), now)
	got := b.Flush()
	if len(got) != 1 || !got[0].Info.Discontinuity || got[0].Info.Missing {
		t.Fatal("a long discontinuity generated invented audio")
	}
}

func TestBoundaryFenceAndRetiredSource(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1, 0, 960), now)
	b.Flush()
	b.Push([]byte{3}, testInfo("a", 3, 1920, 960), now)
	b.Discard()
	if got := b.Push([]byte{3}, testInfo("a", 3, 1920, 960), now); len(got) != 0 {
		t.Fatal("discarded old-route packet crossed the sequence fence")
	}
	got := b.Push([]byte{4}, testInfo("a", 4, 48000, 960), now)
	if len(got) != 1 || got[0].Info.Missing {
		t.Fatal("stream boundary silence was treated as loss")
	}
	b.Push([]byte{1}, testInfo("b", 1, 0, 960), now)
	if got := b.Push([]byte{5}, testInfo("a", 5, 48960, 960), now); len(got) != 0 {
		t.Fatal("retired source was resurrected")
	}
	got = b.Flush()
	if len(got) != 1 || got[0].Info.SourceID != "b" {
		t.Fatal("sources shared pending audio")
	}
}

func TestOverflowDoesNotRewindOrGrow(t *testing.T) {
	now := time.Unix(100, 0)
	size, ok := retainedBytes(2, testInfo("a", 10, 0, 960), 128*1024)
	if !ok {
		t.Fatal("ordinary frame did not fit the default byte budget")
	}
	b := New(Config{MaxPackets: 2, MaxBytes: size * 2})
	b.Push([]byte{1, 1}, testInfo("a", 10, 0, 960), now)
	b.Push([]byte{2, 2}, testInfo("a", 11, 960, 960), now)
	got := b.Push([]byte{0, 0}, testInfo("a", 9, 0, 960), now)
	if len(got) != 2 || len(b.pending) != 0 || b.next != 12 || b.bytes != 0 {
		t.Fatal("overflow rewound the media timeline or retained frame bytes")
	}
}

func requireRealSequences(t *testing.T, frames []Frame, sequences ...uint16) {
	t.Helper()
	if len(frames) != len(sequences) {
		t.Fatalf("got %d frames, want sequences %v: %+v", len(frames), sequences, frames)
	}
	for i, seq := range sequences {
		if frames[i].Info == nil || frames[i].Info.Missing || len(frames[i].Payload) == 0 || frames[i].Info.Header.SequenceNumber != seq {
			t.Fatalf("frame %d = %+v, want real sequence %d", i, frames[i], seq)
		}
	}
}

func TestDescendingArrivalsKeepGapDeadline(t *testing.T) {
	// A zero arrival time is valid evidence, not an unset-deadline sentinel.
	now := time.Time{}
	for _, test := range []struct {
		name      string
		secondAt  time.Duration
		thirdAt   time.Duration
		releaseAt time.Duration
	}{
		{"within window", 19 * time.Millisecond, 39 * time.Millisecond, 40 * time.Millisecond},
		{"overdue push", 39 * time.Millisecond, 78 * time.Millisecond, 78 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := New(Config{})
			b.Push([]byte{10}, testInfo("a", 10, 0, 960), now.Add(-40*time.Millisecond))
			requireRealSequences(t, b.Pop(now), 10)
			b.Push([]byte{14}, testInfo("a", 14, 3840, 960), now)
			b.Push([]byte{13}, testInfo("a", 13, 2880, 960), now.Add(test.secondAt))
			if deadline, ok := b.Deadline(); !ok || !deadline.Equal(now.Add(40*time.Millisecond)) {
				t.Fatalf("lower sequence moved the open gap deadline: %v, %v", deadline, ok)
			}
			got := b.Push([]byte{12}, testInfo("a", 12, 1920, 960), now.Add(test.thirdAt))
			if test.releaseAt != test.thirdAt {
				if len(got) != 0 {
					t.Fatal("packet escaped before the fixed deadline")
				}
				if deadline, ok := b.Deadline(); !ok || !deadline.Equal(now.Add(40*time.Millisecond)) {
					t.Fatalf("third descending arrival moved deadline: %v, %v", deadline, ok)
				}
				got = b.Pop(now.Add(test.releaseAt))
			}
			if len(got) != 4 || !got[0].Info.Missing || got[0].Info.Duration48 != 960 {
				t.Fatalf("gap did not release at %v: %+v", test.releaseAt, got)
			}
			requireRealSequences(t, got[1:], 12, 13, 14)
			if _, ok := b.Deadline(); ok || b.bytes != 0 {
				t.Fatal("released frames retained a deadline or byte charge")
			}
		})
	}
}

func TestGapDeadlineTimerReleasesBeforeLaterIntermediate(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{10}, testInfo("a", 10, 0, 960), now.Add(-40*time.Millisecond))
	b.Pop(now)
	b.Push([]byte{14}, testInfo("a", 14, 3840, 960), now)
	b.Push([]byte{13}, testInfo("a", 13, 2880, 960), now.Add(39*time.Millisecond))
	got := b.Pop(now.Add(40 * time.Millisecond))
	if len(got) != 3 || !got[0].Info.Missing || got[0].Info.Duration48 != 1920 {
		t.Fatalf("timer did not release the original gap: %+v", got)
	}
	requireRealSequences(t, got[1:], 13, 14)
	if got := b.Push([]byte{12}, testInfo("a", 12, 1920, 960), now.Add(78*time.Millisecond)); len(got) != 0 {
		t.Fatal("intermediate packet crossed the timer's delivered fence")
	}
}

func TestInitialDescendingArrivalsKeepPrimingDeadline(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{14}, testInfo("a", 14, 3840, 960), now)
	b.Push([]byte{13}, testInfo("a", 13, 2880, 960), now.Add(39*time.Millisecond))
	if deadline, ok := b.Deadline(); !ok || !deadline.Equal(now.Add(40*time.Millisecond)) {
		t.Fatal("priming deadline moved with a lower sequence")
	}
	requireRealSequences(t, b.Pop(now.Add(40*time.Millisecond)), 13, 14)
}

func TestExpectedSequenceImmediatelyFillsGap(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{10}, testInfo("a", 10, 0, 960), now)
	b.Flush()
	b.Push([]byte{13}, testInfo("a", 13, 2880, 960), now)
	b.Push([]byte{12}, testInfo("a", 12, 1920, 960), now.Add(time.Millisecond))
	requireRealSequences(t, b.Push([]byte{11}, testInfo("a", 11, 960, 960), now.Add(2*time.Millisecond)), 11, 12, 13)
}

func TestExplicitDiscontinuityResetsSameSourceFence(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
	b.Flush()
	b.Push([]byte{3}, testInfo("a", 1002, 1920, 960), now)
	info := testInfo("a", 1, 0, 960)
	info.Discontinuity = true
	if got := b.Push([]byte{9}, info, now); len(got) != 0 {
		t.Fatal("explicit restart bypassed priming")
	}
	got := b.Pop(now.Add(40 * time.Millisecond))
	requireRealSequences(t, got, 1)
	if !got[0].Info.Discontinuity || len(b.pending) != 0 || b.bytes != 0 {
		t.Fatal("explicit restart retained old media or lost its discontinuity")
	}
	requireRealSequences(t, b.Push([]byte{2}, testInfo("a", 2, 960, 960), now.Add(41*time.Millisecond)), 2)
}

func TestResetAndDiscardHaveDistinctSequenceFences(t *testing.T) {
	now := time.Unix(100, 0)
	for _, reset := range []bool{false, true} {
		name := "discard"
		if reset {
			name = "reset"
		}
		t.Run(name, func(t *testing.T) {
			b := New(Config{})
			b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
			b.Flush()
			b.Push([]byte{3}, testInfo("a", 1002, 1920, 960), now)
			if reset {
				b.Reset()
			} else {
				b.Discard()
			}
			if b.bytes != 0 || len(b.pending) != 0 || b.clock {
				t.Fatal("boundary retained media or the concealment clock")
			}
			b.Push([]byte{2}, testInfo("a", 1001, 960, 960), now)
			got := b.Flush()
			if reset {
				requireRealSequences(t, got, 1001)
			} else if len(got) != 0 || b.next != 1003 {
				t.Fatal("ordinary boundary dropped its discarded-sequence fence")
			}
		})
	}
}

func TestResetPreservesRetiredSourceFence(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1, 0, 960), now)
	b.Flush()
	b.Push([]byte{1}, testInfo("b", 1, 0, 960), now)
	b.Reset()
	b.Push([]byte{2}, testInfo("b", 2, 960, 960), now)
	old := testInfo("a", 2, 960, 960)
	old.Discontinuity = true
	if got := b.Push([]byte{2}, old, now); len(got) != 0 {
		t.Fatal("explicit restart resurrected a retired source")
	}
	got := b.Flush()
	requireRealSequences(t, got, 2)
	if got[0].Info.SourceID != "b" {
		t.Fatal("retired discontinuity displaced the current source")
	}
}

func TestLongLossBeyondHalfSpaceResynchronizes(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
	b.Flush()
	if got := b.Push([]byte{2}, testInfo("a", 41001, 38400960, 960), now); len(got) != 0 || len(b.pending) != 0 || b.bytes != 0 {
		t.Fatal("first implausible jump retained or emitted a packet")
	}
	if _, ok := b.Deadline(); ok {
		t.Fatal("scalar probation created a media deadline")
	}
	if got := b.Push([]byte{3}, testInfo("a", 41002, 38401920, 960), now.Add(20*time.Millisecond)); len(got) != 0 {
		t.Fatal("confirmed restart bypassed its priming window")
	}
	if got := b.Pop(now.Add(59 * time.Millisecond)); len(got) != 0 {
		t.Fatal("restart priming ended early")
	}
	got := b.Pop(now.Add(60 * time.Millisecond))
	requireRealSequences(t, got, 41002)
	if !got[0].Info.Discontinuity {
		t.Fatal("confirmed long-loss restart did not reset decoder history")
	}
	if got := b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now.Add(61*time.Millisecond)); len(got) != 0 {
		t.Fatal("old packet crossed the resynchronized fence")
	}
	got = b.Push([]byte{4}, testInfo("a", 41003, 38402880, 960), now.Add(61*time.Millisecond))
	requireRealSequences(t, got, 41003)
	if got[0].Info.Discontinuity {
		t.Fatal("ordinary packet restarted a confirmed media clock")
	}
}

func TestRestartProbationRejectsUnconfirmedJumps(t *testing.T) {
	now := time.Unix(100, 0)
	for _, test := range []struct {
		name      string
		seq       uint16
		timestamp uint32
		delay     time.Duration
	}{
		{"duplicate", 41001, 38400960, time.Millisecond},
		{"nonconsecutive", 41003, 38402880, time.Millisecond},
		{"timestamp mismatch", 41002, 38402040, time.Millisecond},
		{"stale clock", 41002, 0, time.Millisecond},
		{"expired", 41002, 38401920, 61 * time.Millisecond},
		{"backwards time", 41002, 38401920, -time.Millisecond},
		{"ordinary late", 999, 0, time.Millisecond},
		{"delivered duplicate", 1000, 0, time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := New(Config{})
			b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
			b.Flush()
			b.Push([]byte{2}, testInfo("a", 41001, 38400960, 960), now)
			if got := b.Push([]byte{3}, testInfo("a", test.seq, test.timestamp, 960), now.Add(test.delay)); len(got) != 0 || len(b.pending) != 0 || b.bytes != 0 || b.next != 1001 {
				t.Fatal("unconfirmed jump changed the delivered fence or retained payload")
			}
			got := b.Push([]byte{4}, testInfo("a", 1001, 960, 960), now.Add(100*time.Millisecond))
			requireRealSequences(t, got, 1001)
			if got[0].Info.Discontinuity {
				t.Fatal("failed probation corrupted the ordinary stream")
			}
		})
	}
}

func TestRestartProbationWrapAndMaximumPacketDuration(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1000, ^uint32(0)-959, 960), now)
	b.Flush()
	b.Push([]byte{2}, testInfo("a", 65535, 64000000, 5760), now)
	b.Push([]byte{3}, testInfo("a", 0, 64005760, 5760), now.Add(120*time.Millisecond))
	got := b.Pop(now.Add(160 * time.Millisecond))
	requireRealSequences(t, got, 0)
	if !got[0].Info.Discontinuity {
		t.Fatal("wrapped, timestamp-confirmed restart did not resynchronize")
	}
}

func TestRestartProbeDuplicateDoesNotExtendDeadline(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
	b.Flush()
	b.Push([]byte{2}, testInfo("a", 41001, 38400960, 960), now)
	b.Push([]byte{2}, testInfo("a", 41001, 38400960, 960), now.Add(59*time.Millisecond))
	b.Push([]byte{3}, testInfo("a", 41002, 38401920, 960), now.Add(61*time.Millisecond))
	if len(b.pending) != 0 || b.bytes != 0 || b.next != 1001 {
		t.Fatal("duplicate extended expired restart probation")
	}
	b.Push([]byte{4}, testInfo("a", 41003, 38402880, 960), now.Add(81*time.Millisecond))
	got := b.Pop(now.Add(121 * time.Millisecond))
	requireRealSequences(t, got, 41003)
	if !got[0].Info.Discontinuity {
		t.Fatal("fresh consecutive evidence did not recover after expired probation")
	}
}

func TestUnconfirmedRestartDoesNotAccumulatePayloads(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
	b.Flush()
	for i := range 1000 {
		seq := uint16(41001 + i*2)
		info := testInfo("a", seq, uint32(38400960+i*1920), 960)
		if got := b.Push([]byte{2}, info, now.Add(time.Duration(i)*time.Millisecond)); len(got) != 0 || len(b.pending) != 0 || b.bytes != 0 || b.next != 1001 {
			t.Fatalf("unconfirmed restart %d retained media or advanced its fence", i)
		}
	}
	if _, ok := b.Deadline(); ok {
		t.Fatal("unconfirmed restart traffic retained a media deadline")
	}
}

func TestDiscardClearsRestartProbation(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	b.Push([]byte{1}, testInfo("a", 1000, 0, 960), now)
	b.Flush()
	b.Push([]byte{2}, testInfo("a", 41001, 38400960, 960), now)
	b.Discard()
	if got := b.Push([]byte{3}, testInfo("a", 41002, 38401920, 960), now.Add(time.Millisecond)); len(got) != 0 || len(b.pending) != 0 || b.next != 1001 {
		t.Fatal("probation crossed an ordinary stream boundary")
	}
	got := b.Push([]byte{4}, testInfo("a", 1001, 48000, 960), now.Add(2*time.Millisecond))
	requireRealSequences(t, got, 1001)
	if got[0].Info.Discontinuity {
		t.Fatal("boundary silence was confused with a large-jump restart")
	}
}

func TestGapDurationMustFitMissingPacketCount(t *testing.T) {
	now := time.Unix(100, 0)
	for _, test := range []struct {
		name    string
		missing uint16
		delta   uint32
		conceal bool
	}{
		{"minimum legal", 1, 120, true},
		{"maximum legal", 1, 5760, true},
		{"two variable packets", 2, 6720, true},
		{"too short for count", 2, 120, false},
		{"DTX plus one lost packet", 1, 23040, false},
		{"above packet maximum", 1, 5880, false},
		{"unaligned", 1, 961, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := New(Config{})
			b.Push([]byte{10}, testInfo("a", 10, 0, 960), now)
			b.Flush()
			seq := uint16(11) + test.missing
			b.Push([]byte{12}, testInfo("a", seq, 960+test.delta, 960), now)
			got := b.Pop(now.Add(40 * time.Millisecond))
			if test.conceal {
				if len(got) != 2 || !got[0].Info.Missing || got[0].Info.Duration48 != test.delta || len(got[0].Payload) != 0 {
					t.Fatalf("legal missing interval was not concealed exactly: %+v", got)
				}
				requireRealSequences(t, got[1:], seq)
				if got[1].Info.Discontinuity {
					t.Fatal("legal gap reset decoder history")
				}
			} else {
				requireRealSequences(t, got, seq)
				if !got[0].Info.Discontinuity {
					t.Fatal("impossible missing duration did not reset decoder history")
				}
			}
		})
	}
}

func TestConcealmentBudgetAcrossPendingGaps(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{MaxGap48: 1920})
	b.Push([]byte{10}, testInfo("a", 10, 0, 960), now)
	b.Flush()
	for _, seq := range []uint16{12, 14, 16} {
		b.Push([]byte{1}, testInfo("a", seq, uint32(seq-10)*960, 960), now)
	}
	got := b.Pop(now.Add(40 * time.Millisecond))
	if len(got) != 5 || !got[0].Info.Missing || !got[2].Info.Missing || got[0].Info.Duration48+got[2].Info.Duration48 != 1920 {
		t.Fatalf("concealment did not honor the aggregate sample budget: %+v", got)
	}
	requireRealSequences(t, []Frame{got[1], got[3], got[4]}, 12, 14, 16)
	if !got[4].Info.Discontinuity || b.bytes != 0 {
		t.Fatal("over-budget gap synthesized media or retained frame storage")
	}
}

func opaqueHeader(t *testing.T, bytes int) rtp.Header {
	t.Helper()
	h := rtp.Header{Version: 2, Extension: true, ExtensionProfile: 0x1234}
	if err := h.SetExtension(0, make([]byte, bytes)); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestRetainedHeaderBudgetRejectsBeforeClone(t *testing.T) {
	now := time.Unix(100, 0)
	opaque := opaqueHeader(t, 61440)
	small := rtp.Header{Extension: true, ExtensionProfile: rtp.ExtensionProfileTwoByte}
	if err := small.SetExtension(1, []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	descriptors := small
	descriptors.Extensions = make([]rtp.Extension, 4096)
	for i := range descriptors.Extensions {
		descriptors.Extensions[i] = small.Extensions[0]
	}
	// Duplicate IDs do not imply duplicate payload sizes. Public ID lookup
	// would see only the first small payload, while Clone copies both entries.
	duplicate := opaqueHeader(t, 4)
	duplicate.Extensions = append(duplicate.Extensions, opaque.Extensions...)
	disabled := opaque
	disabled.Extension = false
	for _, test := range []struct {
		name   string
		header rtp.Header
		limit  int
	}{
		{"opaque payload", opaque, 1024},
		{"duplicate ID payload", duplicate, 1024},
		{"extension descriptors", descriptors, 128 * 1024},
		{"disabled extension payload", disabled, 1024},
		{"CSRC storage", rtp.Header{CSRC: make([]uint32, 4096)}, 1024},
	} {
		t.Run(test.name, func(t *testing.T) {
			b := New(Config{MaxBytes: test.limit})
			info := testInfo("a", 10, 0, 960)
			info.Header = test.header
			info.Header.Version = 2
			info.Header.SequenceNumber = 10
			payload := []byte{1}
			allocations := testing.AllocsPerRun(10, func() {
				if got := b.Push(payload, info, now); len(got) != 0 {
					t.Fatal("oversized header escaped its byte budget")
				}
			})
			if allocations != 0 || len(b.pending) != 0 || b.bytes != 0 || b.priming {
				t.Fatalf("rejection cloned or retained metadata: allocations=%v bytes=%d pending=%d", allocations, b.bytes, len(b.pending))
			}
		})
	}
}

func TestOpaqueExtensionsConsumeAggregateByteBudget(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	header := opaqueHeader(t, 61440)
	for _, seq := range []uint16{10, 11} {
		info := testInfo("a", seq, uint32(seq-10)*960, 960)
		info.Header.Extension = header.Extension
		info.Header.ExtensionProfile = header.ExtensionProfile
		info.Header.Extensions = header.Extensions
		if got := b.Push([]byte{1}, info, now); len(got) != 0 {
			t.Fatal("two opaque headers did not fit the default byte budget")
		}
	}
	if b.bytes < 2*61440 || b.bytes > b.cfg.MaxBytes || len(b.pending) != 2 {
		t.Fatalf("opaque header payload was not charged: bytes=%d pending=%d", b.bytes, len(b.pending))
	}
	info := testInfo("a", 12, 1920, 960)
	info.Header.Extension, info.Header.ExtensionProfile, info.Header.Extensions = header.Extension, header.ExtensionProfile, header.Extensions
	got := b.Push([]byte{2}, info, now)
	requireRealSequences(t, got, 10, 11, 12)
	if len(b.pending) != 0 || b.bytes != 0 {
		t.Fatal("header byte overflow did not release its retained storage")
	}
}

func TestRetainedFrameOwnsPayloadAndHeaderStorage(t *testing.T) {
	now := time.Unix(100, 0)
	b := New(Config{})
	payload := []byte{1, 2, 3}
	info := testInfo("a", 10, 0, 960)
	info.Header.CSRC = []uint32{9}
	extension := []byte{4, 5}
	if err := info.Header.SetExtension(1, extension); err != nil {
		t.Fatal(err)
	}
	b.Push(payload, info, now)
	charge := b.bytes
	payload[0], extension[0], info.Header.CSRC[0] = 99, 99, 99
	info.Header.SequenceNumber = 100
	got := b.Pop(now.Add(40 * time.Millisecond))
	requireRealSequences(t, got, 10)
	if got[0].Payload[0] != 1 || got[0].Info.Header.CSRC[0] != 9 || got[0].Info.Header.GetExtension(1)[0] != 4 {
		t.Fatal("retained packet aliases caller-owned payload or RTP metadata")
	}
	if charge <= len(payload)+len(extension) || b.bytes != 0 {
		t.Fatal("metadata charge was absent or failed to release with the frame")
	}
}
