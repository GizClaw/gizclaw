package codecconv

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"strconv"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/audio/resampler"
)

// ParsePCM16Format reads the rate and channels of signed PCM16 little-endian
// audio. Missing parameters use fallback, as unqualified PCM does not declare a
// different format. It accepts the PCM MIME names used by GenX audio streams.
func ParsePCM16Format(mimeType string, fallback resampler.Format) (resampler.Format, error) {
	base, params, err := mime.ParseMediaType(mimeType)
	if err != nil {
		return resampler.Format{}, fmt.Errorf("parse PCM MIME: %w", err)
	}
	switch strings.ToLower(base) {
	case "audio/pcm", "audio/x-pcm", "audio/l16":
	default:
		return resampler.Format{}, fmt.Errorf("unsupported PCM MIME %q", mimeType)
	}
	format := fallback
	if value := params["rate"]; value != "" {
		format.SampleRate, err = strconv.Atoi(value)
		if err != nil {
			return resampler.Format{}, fmt.Errorf("invalid PCM rate %q: %w", value, err)
		}
	}
	if format.SampleRate <= 0 || format.SampleRate > 1<<31-1 {
		return resampler.Format{}, fmt.Errorf("invalid PCM rate %d", format.SampleRate)
	}
	if value := params["channels"]; value != "" {
		switch value {
		case "1":
			format.Stereo = false
		case "2":
			format.Stereo = true
		default:
			return resampler.Format{}, fmt.Errorf("unsupported PCM channels %q", value)
		}
	}
	if value := strings.ToLower(params["format"]); value != "" && value != "s16le" && value != "pcm_s16le" {
		return resampler.Format{}, fmt.Errorf("unsupported PCM sample format %q", value)
	}
	if value := params["bits"]; value != "" && value != "16" {
		return resampler.Format{}, fmt.Errorf("unsupported PCM bits %q", value)
	}
	return format, nil
}

// PCM16Converter converts one continuous PCM route to a fixed target format.
// Calls are serialized by its owner. Convert retains fractional source frames
// and FIR state; Flush drains them at successful EOF. Close discards a canceled
// route without synthesizing a tail.
type PCM16Converter struct {
	target  resampler.Format
	source  resampler.Format
	input   pcm16ConverterInput
	reader  resampler.Resampler
	scratch []byte
	closed  bool
}

type pcm16ConverterInput struct {
	bytes.Buffer
	finished bool
}

func (r *pcm16ConverterInput) Read(p []byte) (int, error) {
	if r.Len() == 0 && !r.finished {
		return 0, nil
	}
	return r.Buffer.Read(p)
}

// NewPCM16Converter creates a converter whose source format comes from each
// Convert call's MIME parameters. A route cannot change its source format.
func NewPCM16Converter(target resampler.Format) *PCM16Converter {
	return &PCM16Converter{target: target}
}

// Convert consumes the next source bytes, returning all currently available
// target PCM. A short input chunk need not produce output yet.
func (c *PCM16Converter) Convert(mimeType string, data []byte) ([]byte, error) {
	if c.closed || c.input.finished {
		return nil, io.ErrClosedPipe
	}
	source, err := ParsePCM16Format(mimeType, c.target)
	if err != nil {
		return nil, err
	}
	if c.source.SampleRate != 0 && source != c.source {
		return nil, fmt.Errorf("PCM source format changed within a route")
	}
	c.source = source
	if c.reader == nil {
		frameBytes := 2
		if source.Stereo {
			frameBytes = 4
		}
		if source == c.target && len(data)%frameBytes == 0 {
			return data, nil
		}
		c.reader, err = resampler.New(&c.input, source, c.target)
		if err != nil {
			return nil, err
		}
	}
	_, _ = c.input.Write(data)
	return c.drain()
}

// Flush finishes this route once and returns its remaining target PCM.
func (c *PCM16Converter) Flush() ([]byte, error) {
	if c.closed || c.input.finished {
		return nil, nil
	}
	c.input.finished = true
	if c.reader == nil {
		return nil, nil
	}
	return c.drain()
}

func (c *PCM16Converter) drain() ([]byte, error) {
	if c.scratch == nil {
		c.scratch = make([]byte, 4096)
	}
	var out bytes.Buffer
	for {
		n, err := c.reader.Read(c.scratch)
		_, _ = out.Write(c.scratch[:n])
		if errors.Is(err, io.EOF) {
			return out.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 && !c.input.finished && c.input.Len() == 0 {
			// FIR lookahead can consume source frames without emitting output.
			// Pause only when more live input is needed; a finished source must
			// be read through EOF so its filter tail is drained exactly once.
			return out.Bytes(), nil
		}
	}
}

// Close discards buffered input and releases the resampler. It is idempotent.
func (c *PCM16Converter) Close() error {
	if c == nil || c.closed {
		return nil
	}
	c.closed = true
	c.input.Reset()
	c.scratch = nil
	if c.reader != nil {
		return c.reader.Close()
	}
	return nil
}
