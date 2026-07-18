package pcm

import (
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	// L16Mono16K represents audio/L16; rate=16000; channels=1.
	L16Mono16K Format = iota
	// L16Mono24K represents audio/L16; rate=24000; channels=1.
	L16Mono24K
	// L16Mono48K represents audio/L16; rate=48000; channels=1.
	L16Mono48K
	// L16Stereo16K represents audio/L16; rate=16000; channels=2.
	L16Stereo16K
	// L16Stereo24K represents audio/L16; rate=24000; channels=2.
	L16Stereo24K
	// L16Stereo48K represents audio/L16; rate=48000; channels=2.
	L16Stereo48K
)

// L16Format returns the supported signed 16-bit PCM format for sampleRate and channels.
func L16Format(sampleRate, channels int) (Format, error) {
	switch {
	case sampleRate == 16000 && channels == 1:
		return L16Mono16K, nil
	case sampleRate == 24000 && channels == 1:
		return L16Mono24K, nil
	case sampleRate == 48000 && channels == 1:
		return L16Mono48K, nil
	case sampleRate == 16000 && channels == 2:
		return L16Stereo16K, nil
	case sampleRate == 24000 && channels == 2:
		return L16Stereo24K, nil
	case sampleRate == 48000 && channels == 2:
		return L16Stereo48K, nil
	default:
		return 0, fmt.Errorf("pcm: unsupported L16 format rate=%d channels=%d", sampleRate, channels)
	}
}

// Chunk is a chunk of audio data.
type Chunk interface {
	Len() int64
	Format() Format
	WriteTo(w io.Writer) (int64, error)
}

// Format represents an audio format configuration.
type Format int

// SampleRate returns the sample rate in Hz for this format.
func (f Format) SampleRate() int {
	switch f {
	case L16Mono16K, L16Stereo16K:
		return 16000
	case L16Mono24K, L16Stereo24K:
		return 24000
	case L16Mono48K, L16Stereo48K:
		return 48000
	}
	panic("pcm: invalid audio type")
}

// Channels returns the number of audio channels for this format.
func (f Format) Channels() int {
	switch f {
	case L16Mono16K, L16Mono24K, L16Mono48K:
		return 1
	case L16Stereo16K, L16Stereo24K, L16Stereo48K:
		return 2
	}
	panic("pcm: invalid audio type")
}

// Depth returns the bit depth for this format.
func (f Format) Depth() int {
	switch f {
	case L16Mono16K, L16Mono24K, L16Mono48K, L16Stereo16K, L16Stereo24K, L16Stereo48K:
		return 16
	}
	panic("pcm: invalid audio type")
}

// Samples returns the number of samples in the given number of bytes.
func (f Format) Samples(bytes int64) int64 {
	return bytes * 8 / int64(f.Channels()) / int64(f.Depth())
}

// SamplesInDuration returns the number of samples in the given duration.
func (f Format) SamplesInDuration(d time.Duration) int64 {
	return int64(time.Duration(f.SampleRate()) * d / time.Second)
}

// BytesInDuration returns the number of bytes in the given duration.
func (f Format) BytesInDuration(d time.Duration) int64 {
	return f.SamplesInDuration(d) * int64(f.Channels()) * int64(f.Depth()) / 8
}

// Duration returns the duration of the given number of bytes.
func (f Format) Duration(bytes int64) time.Duration {
	return time.Duration(f.Samples(bytes)) * time.Second / time.Duration(f.SampleRate())
}

// BitsRate returns the bit rate of the audio data.
func (f Format) BitsRate() int {
	return f.SampleRate() * f.Channels() * f.Depth()
}

// BytesRate returns the byte rate of the audio data.
func (f Format) BytesRate() int {
	return f.BitsRate() / 8
}

// SilenceChunk returns a silence chunk of the given duration.
func (f Format) SilenceChunk(duration time.Duration) Chunk {
	return &SilenceChunk{
		Duration: duration,
		len:      f.BytesInDuration(duration),
		fmt:      f,
	}
}

// DataChunk returns a chunk of audio data.
func (f Format) DataChunk(data []byte) Chunk {
	return &DataChunk{
		Data: data,
		fmt:  f,
	}
}

// ReadChunk reads exactly the given duration of audio data from the reader.
func (f Format) ReadChunk(r io.Reader, duration time.Duration) (Chunk, error) {
	buf := make([]byte, f.BytesInDuration(duration))
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	return f.DataChunk(buf), nil
}

// String returns a human-readable string representation of the format.
func (f Format) String() string {
	switch f {
	case L16Mono16K:
		return "audio/L16; rate=16000; channels=1"
	case L16Mono24K:
		return "audio/L16; rate=24000; channels=1"
	case L16Mono48K:
		return "audio/L16; rate=48000; channels=1"
	case L16Stereo16K:
		return "audio/L16; rate=16000; channels=2"
	case L16Stereo24K:
		return "audio/L16; rate=24000; channels=2"
	case L16Stereo48K:
		return "audio/L16; rate=48000; channels=2"
	}
	panic("pcm: invalid audio type")
}

// DataChunk is a chunk of audio data.
type DataChunk struct {
	Data []byte
	fmt  Format
}

// Len returns the length of the audio data in bytes.
func (c *DataChunk) Len() int64 {
	return int64(len(c.Data))
}

// Format returns the audio format of this chunk.
func (c *DataChunk) Format() Format {
	return c.fmt
}

// ReadFrom reads audio data from the reader into this chunk.
func (c *DataChunk) ReadFrom(r io.Reader) (int64, error) {
	n, err := r.Read(c.Data[:cap(c.Data)])
	c.Data = c.Data[:n]

	if err != nil {
		if errors.Is(err, io.EOF) {
			return int64(n), nil
		}
		return int64(n), err
	}
	return int64(n), nil
}

// WriteTo writes the audio data to the writer.
func (c *DataChunk) WriteTo(w io.Writer) (int64, error) {
	return writeAll(w, c.Data)
}

// SilenceChunk is a chunk of silence.
type SilenceChunk struct {
	Duration time.Duration
	len      int64
	fmt      Format
}

// Len returns the length of the silence in bytes.
func (c *SilenceChunk) Len() int64 {
	return c.len
}

// Format returns the audio format of this chunk.
func (c *SilenceChunk) Format() Format {
	return c.fmt
}

var emptyBytes [32000]byte

// WriteTo writes silence (zero bytes) to the writer.
func (c *SilenceChunk) WriteTo(w io.Writer) (int64, error) {
	tw := c.len
	wn := int64(0)
	for tw > 0 {
		var silence []byte
		if tw > int64(len(emptyBytes)) {
			silence = emptyBytes[:]
		} else {
			silence = emptyBytes[:tw]
		}

		n, err := writeAll(w, silence)
		wn += n
		if err != nil {
			return wn, err
		}
		tw -= int64(len(silence))
	}
	return wn, nil
}

func writeAll(w io.Writer, p []byte) (int64, error) {
	if len(p) == 0 {
		return 0, nil
	}

	written := int64(0)
	for len(p) > 0 {
		n, err := w.Write(p)
		if n > 0 {
			written += int64(n)
			p = p[n:]
		}
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}

	return written, nil
}
