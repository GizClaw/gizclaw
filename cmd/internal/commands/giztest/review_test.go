package giztest

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReviewRequiresExplicitPass(t *testing.T) {
	var output bytes.Buffer
	if err := runReview(strings.NewReader("pass\n"), &output, "check"); err != nil {
		t.Fatal(err)
	}
	if err := runReview(strings.NewReader("fail\n"), &output, "check"); err == nil {
		t.Fatal("failed review accepted")
	}
}
