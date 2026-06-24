package petspecies

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

const (
	defaultListLimit = 50
	maxListLimit     = 200

	pixaHeaderSize     = 40
	pixaClipEntrySize  = 56
	pixaClipNameSize   = 32
	pixaFrameEntrySize = 16
)

var rootKey = kv.Key{"by-id"}

type Server struct {
	Store  kv.Store
	Assets objectstore.ObjectStore
	Now    func() time.Time
}

func (s *Server) Put(ctx context.Context, id string, spec apitypes.PetSpeciesSpec) (apitypes.PetSpecies, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.PetSpecies{}, errors.New("pet species id is required")
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return apitypes.PetSpecies{}, errors.New("pet species name is required")
	}
	now := s.now()
	current, err := Get(ctx, store, id)
	if err != nil && !errors.Is(err, kv.ErrNotFound) {
		return apitypes.PetSpecies{}, err
	}
	out := apitypes.PetSpecies{
		Id:        id,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err == nil {
		out.CreatedAt = current.CreatedAt
		out.PixaPath = current.PixaPath
		out.PixaMetadata = current.PixaMetadata
	}
	if spec.PixaPath != nil {
		out.PixaPath = strings.TrimSpace(*spec.PixaPath)
	}
	if err := Write(ctx, store, out); err != nil {
		return apitypes.PetSpecies{}, err
	}
	return out, nil
}

func (s *Server) Get(ctx context.Context, id string) (apitypes.PetSpecies, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	return Get(ctx, store, id)
}

func (s *Server) Delete(ctx context.Context, id string) (apitypes.PetSpecies, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	item, err := Get(ctx, store, id)
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	if err := store.Delete(ctx, speciesKey(id)); err != nil {
		return apitypes.PetSpecies{}, err
	}
	if item.PixaPath != "" && s.Assets != nil {
		if err := s.Assets.Delete(item.PixaPath); err != nil {
			return apitypes.PetSpecies{}, err
		}
	}
	return item, nil
}

func (s *Server) List(ctx context.Context, cursor string, limit int) ([]apitypes.PetSpecies, bool, *string, error) {
	store, err := s.store()
	if err != nil {
		return nil, false, nil, err
	}
	normalizedCursor, normalizedLimit := normalizeListParams(cursor, limit)
	entries, err := kv.ListAfter(ctx, store, rootKey, cursorAfterKey(normalizedCursor), normalizedLimit+1)
	if err != nil {
		return nil, false, nil, err
	}
	hasNext := len(entries) > normalizedLimit
	if hasNext {
		entries = entries[:normalizedLimit]
	}
	items := make([]apitypes.PetSpecies, 0, len(entries))
	for _, entry := range entries {
		var item apitypes.PetSpecies
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return nil, false, nil, err
		}
		items = append(items, item)
	}
	var next *string
	if hasNext && len(entries) > 0 {
		v := unescapeStoreSegment(entries[len(entries)-1].Key[len(entries[len(entries)-1].Key)-1])
		next = &v
	}
	return items, hasNext, next, nil
}

func (s *Server) UploadPixa(ctx context.Context, id string, r io.Reader) (apitypes.PetSpecies, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	assets, err := s.assets()
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	item, err := Get(ctx, store, id)
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	metadata, err := ParsePixaMetadata(data)
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	if item.PixaPath == "" {
		item.PixaPath = item.Id + ".pixa"
	}
	if err := assets.Put(item.PixaPath, bytes.NewReader(data)); err != nil {
		return apitypes.PetSpecies{}, err
	}
	item.PixaMetadata = metadata
	item.UpdatedAt = s.now()
	if err := Write(ctx, store, item); err != nil {
		return apitypes.PetSpecies{}, err
	}
	return item, nil
}

func (s *Server) DownloadPixa(ctx context.Context, id string) (io.ReadCloser, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	assets, err := s.assets()
	if err != nil {
		return nil, err
	}
	item, err := Get(ctx, store, id)
	if err != nil {
		return nil, err
	}
	if item.PixaPath == "" {
		return nil, fmt.Errorf("pet species %q has no PIXA file", id)
	}
	return assets.Get(item.PixaPath)
}

func Get(ctx context.Context, store kv.Store, id string) (apitypes.PetSpecies, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return apitypes.PetSpecies{}, errors.New("pet species id is required")
	}
	data, err := store.Get(ctx, speciesKey(id))
	if err != nil {
		return apitypes.PetSpecies{}, err
	}
	var item apitypes.PetSpecies
	if err := json.Unmarshal(data, &item); err != nil {
		return apitypes.PetSpecies{}, err
	}
	return item, nil
}

func Write(ctx context.Context, store kv.Store, item apitypes.PetSpecies) error {
	if strings.TrimSpace(item.Id) == "" {
		return errors.New("pet species id is required")
	}
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return store.Set(ctx, speciesKey(item.Id), data)
}

func ParsePixaMetadata(data []byte) (apitypes.PixaMetadata, error) {
	if len(data) < pixaHeaderSize {
		return apitypes.PixaMetadata{}, errors.New("invalid PIXA file: header is too short")
	}
	if !bytes.Equal(data[0:4], []byte("PIXA")) {
		return apitypes.PixaMetadata{}, errors.New("invalid PIXA magic")
	}
	version := binary.LittleEndian.Uint16(data[4:6])
	if version != 1 {
		return apitypes.PixaMetadata{}, fmt.Errorf("unsupported PIXA version %d", version)
	}
	headerSize := binary.LittleEndian.Uint16(data[6:8])
	if headerSize != pixaHeaderSize {
		return apitypes.PixaMetadata{}, fmt.Errorf("invalid PIXA header size %d", headerSize)
	}
	width := binary.LittleEndian.Uint16(data[8:10])
	height := binary.LittleEndian.Uint16(data[10:12])
	colorCount := binary.LittleEndian.Uint16(data[12:14])
	clipCount := binary.LittleEndian.Uint16(data[14:16])
	frameCount := binary.LittleEndian.Uint32(data[16:20])
	paletteOffset := binary.LittleEndian.Uint32(data[20:24])
	clipOffset := binary.LittleEndian.Uint32(data[24:28])
	frameOffset := binary.LittleEndian.Uint32(data[28:32])
	payloadOffset := binary.LittleEndian.Uint32(data[32:36])
	payloadLen := binary.LittleEndian.Uint32(data[36:40])

	if err := requirePixaRange(data, paletteOffset, uint32(colorCount)*2); err != nil {
		return apitypes.PixaMetadata{}, err
	}
	if err := requirePixaRange(data, clipOffset, uint32(clipCount)*uint32(pixaClipEntrySize)); err != nil {
		return apitypes.PixaMetadata{}, err
	}
	if err := requirePixaRange(data, frameOffset, frameCount*uint32(pixaFrameEntrySize)); err != nil {
		return apitypes.PixaMetadata{}, err
	}
	if err := requirePixaRange(data, payloadOffset, payloadLen); err != nil {
		return apitypes.PixaMetadata{}, err
	}

	clipNames := make([]string, 0, clipCount)
	for i := uint16(0); i < clipCount; i++ {
		base := int(clipOffset) + int(i)*pixaClipEntrySize
		rawName := data[base : base+pixaClipNameSize]
		nameLen := bytes.IndexByte(rawName, 0)
		if nameLen < 0 {
			nameLen = len(rawName)
		}
		name := strings.TrimSpace(string(rawName[:nameLen]))
		if name != "" {
			clipNames = append(clipNames, name)
		}
		firstFrame := binary.LittleEndian.Uint32(data[base+36 : base+40])
		clipFrameCount := binary.LittleEndian.Uint32(data[base+40 : base+44])
		if firstFrame > frameCount || clipFrameCount > frameCount-firstFrame {
			return apitypes.PixaMetadata{}, errors.New("invalid PIXA clip frame range")
		}
	}
	for i := uint32(0); i < frameCount; i++ {
		base := int(frameOffset) + int(i)*pixaFrameEntrySize
		framePayloadOffset := binary.LittleEndian.Uint32(data[base+4 : base+8])
		framePayloadLen := binary.LittleEndian.Uint32(data[base+8 : base+12])
		if framePayloadOffset > payloadLen || framePayloadLen > payloadLen-framePayloadOffset {
			return apitypes.PixaMetadata{}, errors.New("invalid PIXA frame payload range")
		}
	}

	return apitypes.PixaMetadata{
		Version:      int(version),
		CanvasWidth:  int(width),
		CanvasHeight: int(height),
		ColorCount:   int(colorCount),
		ClipCount:    int(clipCount),
		FrameCount:   int(frameCount),
		PayloadBytes: int(payloadLen),
		ClipNames:    clipNames,
	}, nil
}

func requirePixaRange(data []byte, offset uint32, length uint32) error {
	if uint64(offset) > uint64(len(data)) || uint64(length) > uint64(len(data))-uint64(offset) {
		return errors.New("invalid PIXA section range")
	}
	return nil
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("pet species service not configured")
	}
	return s.Store, nil
}

func (s *Server) assets() (objectstore.ObjectStore, error) {
	if s == nil || s.Assets == nil {
		return nil, errors.New("pet species asset store not configured")
	}
	return s.Assets, nil
}

func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func speciesKey(id string) kv.Key {
	return append(append(kv.Key{}, rootKey...), escapeStoreSegment(strings.TrimSpace(id)))
}

func normalizeListParams(cursor string, limit int) (string, int) {
	normalizedCursor := escapeStoreSegment(strings.TrimSpace(cursor))
	normalizedLimit := defaultListLimit
	if limit > 0 {
		normalizedLimit = limit
	}
	if normalizedLimit > maxListLimit {
		normalizedLimit = maxListLimit
	}
	return normalizedCursor, normalizedLimit
}

func cursorAfterKey(cursor string) kv.Key {
	if cursor == "" {
		return nil
	}
	return append(append(kv.Key{}, rootKey...), cursor)
}

func escapeStoreSegment(value string) string {
	return url.QueryEscape(value)
}

func unescapeStoreSegment(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}
