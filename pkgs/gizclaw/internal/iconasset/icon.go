package iconasset

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"net/url"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

const (
	MaxBytes         int64 = 2 * 1024 * 1024
	maxPNGDimension        = 4096
	maxPNGPixelCount int64 = 4 * 1024 * 1024
)

var (
	ErrInvalid  = errors.New("invalid icon")
	ErrTooLarge = errors.New("icon exceeds 2 MiB")
)

type Format string

const (
	FormatPixa Format = "pixa"
	FormatPNG  Format = "png"
)

func ParseFormat(value string) (Format, error) {
	format := Format(value)
	switch format {
	case FormatPixa, FormatPNG:
		return format, nil
	default:
		return "", fmt.Errorf("%w: unsupported format %q", ErrInvalid, value)
	}
}

func ObjectName(identity string, format Format) string {
	return url.PathEscape(identity) + "/icon." + string(format)
}

func GameDefObjectName(identity string, format Format) string {
	return "game-defs/" + ObjectName(identity, format)
}

func ReadValidated(r io.Reader, format Format) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: request body required", ErrInvalid)
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxBytes {
		return nil, ErrTooLarge
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: icon is empty", ErrInvalid)
	}
	switch format {
	case FormatPNG:
		if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
			return nil, fmt.Errorf("%w: invalid PNG signature", ErrInvalid)
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid PNG: %v", ErrInvalid, err)
		}
		if config.Width > maxPNGDimension || config.Height > maxPNGDimension {
			return nil, fmt.Errorf(
				"%w: PNG dimensions %dx%d exceed the %d pixel dimension limit",
				ErrInvalid,
				config.Width,
				config.Height,
				maxPNGDimension,
			)
		}
		if pixelCount := int64(config.Width) * int64(config.Height); pixelCount > maxPNGPixelCount {
			return nil, fmt.Errorf("%w: PNG dimensions %dx%d exceed the %d total pixel limit", ErrInvalid, config.Width, config.Height, maxPNGPixelCount)
		}
	case FormatPixa:
		if err := validatePIXA(data); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	default:
		return nil, fmt.Errorf("%w: unsupported format %q", ErrInvalid, format)
	}
	return data, nil
}

func Open(store objectstore.ObjectStore, name string) (io.ReadCloser, int64, error) {
	if store == nil {
		return nil, 0, errors.New("icon store not configured")
	}
	items, err := store.List(name)
	if err != nil {
		return nil, 0, err
	}
	var size int64 = -1
	for _, item := range items {
		if item.Name == name {
			size = item.Size
			break
		}
	}
	if size < 0 {
		return nil, 0, fmt.Errorf("%w: %s", io.EOF, name)
	}
	reader, err := store.Get(name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, fmt.Errorf("%w: %s", io.EOF, name)
		}
		return nil, 0, err
	}
	return reader, size, nil
}

func Slot(icon *apitypes.Icon, format Format) *string {
	if icon == nil {
		return nil
	}
	if format == FormatPixa {
		return icon.Pixa
	}
	return icon.Png
}

func SetSlot(icon *apitypes.Icon, format Format, name *string) *apitypes.Icon {
	out := apitypes.Icon{}
	if icon != nil {
		out = *icon
	}
	if format == FormatPixa {
		out.Pixa = name
	} else {
		out.Png = name
	}
	if out.Pixa == nil && out.Png == nil {
		return nil
	}
	return &out
}

func Equal(a, b *apitypes.Icon) bool {
	return equalString(aSlot(a, FormatPixa), aSlot(b, FormatPixa)) && equalString(aSlot(a, FormatPNG), aSlot(b, FormatPNG))
}

func aSlot(icon *apitypes.Icon, format Format) *string { return Slot(icon, format) }

func equalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func ValidateProjection(current, requested *apitypes.Icon) error {
	if requested == nil || Equal(current, requested) {
		return nil
	}
	return fmt.Errorf("%w: icon object names are managed by the icon API", ErrInvalid)
}

type Locker struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func (l *Locker) Lock(key string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[string]*sync.Mutex{}
	}
	lock := l.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		l.locks[key] = lock
	}
	l.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (l *Locker) LockOwner(identity string) func() {
	unlockPixa := l.Lock(identity + ":" + string(FormatPixa))
	unlockPNG := l.Lock(identity + ":" + string(FormatPNG))
	unlockRecord := l.Lock(identity + ":record")
	return func() {
		unlockRecord()
		unlockPNG()
		unlockPixa()
	}
}

func (l *Locker) LockRecord(identity string) func() {
	return l.Lock(identity + ":record")
}

func validatePIXA(data []byte) error {
	const (
		headerSize     = 40
		clipEntrySize  = 56
		frameEntrySize = 16
	)
	if len(data) < headerSize {
		return errors.New("PIXA header is too short")
	}
	if string(data[:4]) != "PIXA" {
		return errors.New("invalid PIXA magic")
	}
	if version := binary.LittleEndian.Uint16(data[4:6]); version != 1 {
		return fmt.Errorf("unsupported PIXA version %d", version)
	}
	if size := binary.LittleEndian.Uint16(data[6:8]); size != headerSize {
		return fmt.Errorf("invalid PIXA header size %d", size)
	}
	if binary.LittleEndian.Uint16(data[8:10]) == 0 || binary.LittleEndian.Uint16(data[10:12]) == 0 {
		return errors.New("invalid PIXA canvas size")
	}
	colorCount := binary.LittleEndian.Uint16(data[12:14])
	clipCount := binary.LittleEndian.Uint16(data[14:16])
	frameCount := binary.LittleEndian.Uint32(data[16:20])
	paletteOffset := binary.LittleEndian.Uint32(data[20:24])
	clipOffset := binary.LittleEndian.Uint32(data[24:28])
	frameOffset := binary.LittleEndian.Uint32(data[28:32])
	payloadOffset := binary.LittleEndian.Uint32(data[32:36])
	payloadLength := binary.LittleEndian.Uint32(data[36:40])
	for _, item := range []struct {
		offset uint32
		length uint64
		label  string
	}{
		{paletteOffset, uint64(colorCount) * 2, "palette"},
		{clipOffset, uint64(clipCount) * clipEntrySize, "clip table"},
		{frameOffset, uint64(frameCount) * frameEntrySize, "frame table"},
		{payloadOffset, uint64(payloadLength), "payload"},
	} {
		if uint64(item.offset) > uint64(len(data)) || item.length > uint64(len(data))-uint64(item.offset) {
			return fmt.Errorf("invalid PIXA %s range", item.label)
		}
	}
	for i := range int(clipCount) {
		base := int(clipOffset) + i*clipEntrySize
		first := binary.LittleEndian.Uint32(data[base+36 : base+40])
		count := binary.LittleEndian.Uint32(data[base+40 : base+44])
		if count == 0 || first > frameCount || count > frameCount-first {
			return errors.New("invalid PIXA clip frame range")
		}
	}
	for i := range int(frameCount) {
		base := int(frameOffset) + i*frameEntrySize
		offset := binary.LittleEndian.Uint32(data[base+4 : base+8])
		length := binary.LittleEndian.Uint32(data[base+8 : base+12])
		if offset > payloadLength || length > payloadLength-offset {
			return errors.New("invalid PIXA frame payload range")
		}
	}
	return nil
}
