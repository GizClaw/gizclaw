package doubaorealtime

import "testing"

func TestPTTAudioOwnershipSurvivesLateResponseTerminals(t *testing.T) {
	first := &doubaoRealtimePTTResponse{identity: doubaoRealtimePTTResponseIdentity{replyID: "first"}}
	second := &doubaoRealtimePTTResponse{identity: doubaoRealtimePTTResponseIdentity{replyID: "second"}}
	responses := &doubaoRealtimePTTResponses{}
	responses.add(first)
	responses.startAudio(first.identity)
	responses.add(second)
	responses.startAudio(second.identity)
	anonymous := doubaoRealtimePTTResponseIdentity{}
	if got := responses.matchAudio(anonymous); got != second {
		t.Fatal("anonymous audio did not follow the second TTS start")
	}
	// Both old terminal events still match the old response without stealing
	// the audio owner, including after the old response leaves the queue.
	first.ttsFinished = true
	responses.finish(first)
	if got := responses.match(first.identity); got != first {
		t.Fatal("lost late first-response event binding")
	}
	if got := responses.matchAudio(anonymous); got != second {
		t.Fatal("old TTS finish changed the audio owner")
	}
	first.chatEnded = true
	responses.finish(first)
	if got := responses.matchAudio(anonymous); got != second {
		t.Fatal("old ChatEnded changed the audio owner")
	}
	second.ttsFinished = true
	if got := responses.matchAudio(anonymous); got != nil {
		t.Fatal("accepted anonymous audio after TTS finish")
	}
	if got := responses.matchAudio(second.identity); got != nil {
		t.Fatal("accepted identified audio after TTS finish")
	}
}

func TestPTTAudioOwnershipRejectsUnknownAndAmbiguousStarts(t *testing.T) {
	first := &doubaoRealtimePTTResponse{identity: doubaoRealtimePTTResponseIdentity{replyID: "first"}}
	second := &doubaoRealtimePTTResponse{identity: doubaoRealtimePTTResponseIdentity{replyID: "second"}}
	responses := &doubaoRealtimePTTResponses{}
	anonymous := doubaoRealtimePTTResponseIdentity{}
	responses.add(first)
	if got := responses.matchAudio(anonymous); got != first {
		t.Fatal("single response could not receive audio without TTSStarted")
	}
	responses.add(second)
	if got := responses.matchAudio(anonymous); got != nil {
		t.Fatal("guessed audio ownership between two responses without TTSStarted")
	}
	if got := responses.matchAudio(second.identity); got != second {
		t.Fatal("identified audio did not match its response")
	}
	responses.startAudio(first.identity)
	if got := responses.startAudio(doubaoRealtimePTTResponseIdentity{replyID: "unknown"}); got != nil {
		t.Fatal("matched an unknown TTS start")
	}
	if got := responses.matchAudio(anonymous); got != nil {
		t.Fatal("unknown TTS audio leaked into a previous response")
	}
	responses.startAudio(second.identity)
	if got := responses.matchAudio(anonymous); got != second {
		t.Fatal("known TTS start did not restore audio ownership")
	}
}
