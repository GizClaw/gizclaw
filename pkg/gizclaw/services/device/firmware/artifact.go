package firmware

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

const (
	firmwareArtifactTarName      = "artifact.tar"
	firmwareArtifactManifestName = "manifest.json"
	firmwareArtifactFilesDir     = "files"
	firmwareArtifactContentType  = "application/x-tar"
)

var (
	errInvalidArtifact  = errors.New("invalid firmware artifact")
	errArtifactExists   = errors.New("firmware artifact already exists")
	errArtifactNotFound = errors.New("firmware artifact not found")
)

type artifactManifest struct {
	Entries []apitypes.FirmwareArtifactEntry `json:"entries"`
}

func (s *Server) DownloadFirmwareArtifact(ctx context.Context, request adminservice.DownloadFirmwareArtifactRequestObject) (adminservice.DownloadFirmwareArtifactResponseObject, error) {
	_, slot, _, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return downloadArtifactError(err), nil
	}
	if slot.Artifact == nil {
		return adminservice.DownloadFirmwareArtifact404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", errArtifactNotFound.Error())), nil
	}
	assets, err := s.assets()
	if err != nil {
		return adminservice.DownloadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ASSETS_NOT_CONFIGURED", err.Error())), nil
	}
	reader, err := assets.Get(slot.Artifact.TarPath)
	if err != nil {
		return adminservice.DownloadFirmwareArtifact404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error())), nil
	}
	return adminservice.DownloadFirmwareArtifact200ApplicationxTarResponse{Body: reader, ContentLength: slot.Artifact.Size}, nil
}

func (s *Server) UploadFirmwareArtifact(ctx context.Context, request adminservice.UploadFirmwareArtifactRequestObject) (adminservice.UploadFirmwareArtifactResponseObject, error) {
	item, _, channel, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return uploadArtifactError(err), nil
	}
	slot, _ := slotForChannel(&item.Slots, channel)
	if request.Body == nil {
		return adminservice.UploadFirmwareArtifact400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT", "request body required")), nil
	}
	assets, err := s.assets()
	if err != nil {
		return adminservice.UploadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ASSETS_NOT_CONFIGURED", err.Error())), nil
	}
	prefix := firmwareArtifactPrefix(item.Name, channel)
	if exists, err := artifactPrefixHasObjects(assets, prefix); err != nil {
		return adminservice.UploadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	} else if exists || slot.Artifact != nil {
		return adminservice.UploadFirmwareArtifact409JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_EXISTS", errArtifactExists.Error())), nil
	}
	artifact, err := writeArtifactPackage(ctx, assets, item.Name, channel, request.Body, s.now())
	if err != nil {
		if errors.Is(err, errInvalidArtifact) {
			return adminservice.UploadFirmwareArtifact400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT", err.Error())), nil
		}
		return adminservice.UploadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	slot.Artifact = &artifact
	item.UpdatedAt = artifact.UploadedAt
	if err := Write(ctx, s.Store, item); err != nil {
		return adminservice.UploadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.UploadFirmwareArtifact200JSONResponse(item), nil
}

func (s *Server) DeleteFirmwareArtifact(ctx context.Context, request adminservice.DeleteFirmwareArtifactRequestObject) (adminservice.DeleteFirmwareArtifactResponseObject, error) {
	item, _, channel, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return deleteArtifactError(err), nil
	}
	slot, _ := slotForChannel(&item.Slots, channel)
	assets, err := s.assets()
	if err != nil {
		return adminservice.DeleteFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ASSETS_NOT_CONFIGURED", err.Error())), nil
	}
	prefix := firmwareArtifactPrefix(item.Name, channel)
	exists, err := artifactPrefixHasObjects(assets, prefix)
	if err != nil {
		return adminservice.DeleteFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	if slot.Artifact == nil && !exists {
		return adminservice.DeleteFirmwareArtifact404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", errArtifactNotFound.Error())), nil
	}
	if slot.Artifact != nil {
		slot.Artifact = nil
		item.UpdatedAt = s.now()
		if err := Write(ctx, s.Store, item); err != nil {
			return adminservice.DeleteFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
		}
	}
	if err := assets.DeletePrefix(prefix); err != nil {
		return adminservice.DeleteFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.DeleteFirmwareArtifact200JSONResponse(item), nil
}

func (s *Server) ListFirmwareArtifactEntries(ctx context.Context, request adminservice.ListFirmwareArtifactEntriesRequestObject) (adminservice.ListFirmwareArtifactEntriesResponseObject, error) {
	item, slot, channel, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return listArtifactError(err), nil
	}
	target, err := normalizeArtifactPath(valueOrEmpty(request.Params.Path), true)
	if err != nil {
		return adminservice.ListFirmwareArtifactEntries400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error())), nil
	}
	manifest, err := s.readArtifactManifest(slot)
	if err != nil {
		return listArtifactError(err), nil
	}
	items, err := artifactListItems(manifest.Entries, target)
	if err != nil {
		return listArtifactError(err), nil
	}
	return adminservice.ListFirmwareArtifactEntries200JSONResponse(apitypes.FirmwareArtifactList{
		FirmwareId: item.Name,
		Channel:    channel,
		Path:       target,
		Items:      items,
	}), nil
}

func (s *Server) TreeFirmwareArtifactEntries(ctx context.Context, request adminservice.TreeFirmwareArtifactEntriesRequestObject) (adminservice.TreeFirmwareArtifactEntriesResponseObject, error) {
	item, slot, channel, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return treeArtifactError(err), nil
	}
	target, err := normalizeArtifactPath(valueOrEmpty(request.Params.Path), true)
	if err != nil {
		return adminservice.TreeFirmwareArtifactEntries400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error())), nil
	}
	manifest, err := s.readArtifactManifest(slot)
	if err != nil {
		return treeArtifactError(err), nil
	}
	items, err := artifactTreeItems(manifest.Entries, target)
	if err != nil {
		return treeArtifactError(err), nil
	}
	return adminservice.TreeFirmwareArtifactEntries200JSONResponse(apitypes.FirmwareArtifactTree{
		FirmwareId: item.Name,
		Channel:    channel,
		Path:       target,
		Items:      items,
	}), nil
}

func (s *Server) StatFirmwareArtifactEntry(ctx context.Context, request adminservice.StatFirmwareArtifactEntryRequestObject) (adminservice.StatFirmwareArtifactEntryResponseObject, error) {
	item, slot, channel, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return statArtifactError(err), nil
	}
	target, err := normalizeArtifactPath(valueOrEmpty(request.Params.Path), true)
	if err != nil {
		return adminservice.StatFirmwareArtifactEntry400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error())), nil
	}
	manifest, err := s.readArtifactManifest(slot)
	if err != nil {
		return statArtifactError(err), nil
	}
	stats, err := artifactStats(*slot.Artifact, manifest.Entries, target)
	if err != nil {
		return statArtifactError(err), nil
	}
	stats.FirmwareId = item.Name
	stats.Channel = channel
	return adminservice.StatFirmwareArtifactEntry200JSONResponse(stats), nil
}

func (s *Server) DownloadFirmwareArtifactEntry(ctx context.Context, request adminservice.DownloadFirmwareArtifactEntryRequestObject) (adminservice.DownloadFirmwareArtifactEntryResponseObject, error) {
	_, slot, _, err := s.getArtifactSlot(ctx, request.Name, string(request.Channel))
	if err != nil {
		return downloadEntryError(err), nil
	}
	_, entry, reader, err := s.prepareArtifactEntryDownload(slot, request.Params.Path)
	if err != nil {
		return downloadEntryError(err), nil
	}
	return adminservice.DownloadFirmwareArtifactEntry200ApplicationoctetStreamResponse{Body: reader, ContentLength: entry.Size}, nil
}

func (s *Server) PrepareArtifactEntryDownload(ctx context.Context, name, channel, filePath string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error) {
	_, slot, _, err := s.getArtifactSlot(ctx, name, channel)
	if err != nil {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, err
	}
	return s.prepareArtifactEntryDownload(slot, filePath)
}

func (s *Server) prepareArtifactEntryDownload(slot *apitypes.FirmwareSlot, filePath string) (apitypes.FirmwareArtifact, apitypes.FirmwareArtifactEntry, io.ReadCloser, error) {
	target, err := normalizeArtifactPath(filePath, false)
	if err != nil {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, err
	}
	manifest, err := s.readArtifactManifest(slot)
	if err != nil {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, err
	}
	entry, ok := findArtifactEntry(manifest.Entries, target)
	if !ok {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, fmt.Errorf("%w: %s", errArtifactNotFound, target)
	}
	if entry.Type != apitypes.FirmwareArtifactEntryTypeFile {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, fmt.Errorf("%w: path is not a file", errInvalidArtifact)
	}
	assets, err := s.assets()
	if err != nil {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, err
	}
	reader, err := assets.Get(path.Join(slot.Artifact.FilesPath, target))
	if err != nil {
		return apitypes.FirmwareArtifact{}, apitypes.FirmwareArtifactEntry{}, nil, fmt.Errorf("%w: %v", errArtifactNotFound, err)
	}
	return *slot.Artifact, entry, reader, nil
}

func IsInvalidArtifactError(err error) bool {
	return errors.Is(err, errInvalidArtifact)
}

func IsArtifactNotFoundError(err error) bool {
	return errors.Is(err, errArtifactNotFound)
}

func (s *Server) getArtifactSlot(ctx context.Context, rawName, rawChannel string) (apitypes.Firmware, *apitypes.FirmwareSlot, string, error) {
	store, err := s.store()
	if err != nil {
		return apitypes.Firmware{}, nil, "", err
	}
	name, err := url.PathUnescape(rawName)
	if err != nil {
		return apitypes.Firmware{}, nil, "", fmt.Errorf("%w: %v", errInvalidArtifact, err)
	}
	name = strings.TrimSpace(name)
	channel := strings.TrimSpace(rawChannel)
	if name == "" {
		return apitypes.Firmware{}, nil, "", fmt.Errorf("%w: firmware name is required", errInvalidArtifact)
	}
	item, err := Get(ctx, store, name)
	if err != nil {
		return apitypes.Firmware{}, nil, "", err
	}
	slot, ok := slotForChannel(&item.Slots, channel)
	if !ok {
		return apitypes.Firmware{}, nil, "", fmt.Errorf("%w: %s", errChannelNotFound, channel)
	}
	return item, slot, channel, nil
}

func writeArtifactPackage(ctx context.Context, assets objectstore.ObjectStore, name, channel string, body io.Reader, uploadedAt time.Time) (apitypes.FirmwareArtifact, error) {
	prefix := firmwareArtifactPrefix(name, channel)
	tarPath := path.Join(prefix, firmwareArtifactTarName)
	manifestPath := path.Join(prefix, firmwareArtifactManifestName)
	filesPath := path.Join(prefix, firmwareArtifactFilesDir)

	pr, pw := io.Pipe()
	putErrCh := make(chan error, 1)
	go func() {
		putErrCh <- assets.Put(tarPath, pr)
	}()

	hash := sha256.New()
	counter := &byteCounter{}
	tee := io.TeeReader(body, io.MultiWriter(pw, hash, counter))
	entries, err := extractArtifactTar(ctx, assets, filesPath, tar.NewReader(tee))
	if err != nil {
		_ = pw.CloseWithError(err)
		<-putErrCh
		return apitypes.FirmwareArtifact{}, err
	}
	if err := pw.Close(); err != nil {
		<-putErrCh
		return apitypes.FirmwareArtifact{}, err
	}
	if err := <-putErrCh; err != nil {
		return apitypes.FirmwareArtifact{}, err
	}
	manifest := artifactManifest{Entries: entries}
	data, err := json.Marshal(manifest)
	if err != nil {
		return apitypes.FirmwareArtifact{}, err
	}
	if err := assets.Put(manifestPath, bytes.NewReader(data)); err != nil {
		return apitypes.FirmwareArtifact{}, err
	}
	return apitypes.FirmwareArtifact{
		TarPath:      tarPath,
		ManifestPath: manifestPath,
		FilesPath:    filesPath,
		Sha256:       hex.EncodeToString(hash.Sum(nil)),
		Size:         counter.N,
		ContentType:  firmwareArtifactContentType,
		UploadedAt:   uploadedAt.UTC(),
	}, nil
}

func extractArtifactTar(_ context.Context, assets objectstore.ObjectStore, filesPath string, tr *tar.Reader) ([]apitypes.FirmwareArtifactEntry, error) {
	entries := make(map[string]apitypes.FirmwareArtifactEntry)
	hasFile := false
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: read tar: %v", errInvalidArtifact, err)
		}
		entryPath, err := normalizeArtifactPath(header.Name, false)
		if err != nil {
			return nil, err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := addArtifactDir(entries, entryPath, header); err != nil {
				return nil, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if _, exists := entries[entryPath]; exists {
				return nil, fmt.Errorf("%w: duplicate entry %q", errInvalidArtifact, entryPath)
			}
			if err := addArtifactParentDirs(entries, entryPath, header.ModTime); err != nil {
				return nil, err
			}
			entry := apitypes.FirmwareArtifactEntry{
				Path:    entryPath,
				Type:    apitypes.FirmwareArtifactEntryTypeFile,
				Size:    header.Size,
				Mode:    int32(header.Mode),
				ModTime: header.ModTime.UTC(),
			}
			if contentType := mime.TypeByExtension(path.Ext(entryPath)); contentType != "" {
				entry.ContentType = &contentType
			}
			if err := assets.Put(path.Join(filesPath, entryPath), tr); err != nil {
				return nil, err
			}
			entries[entryPath] = entry
			hasFile = true
		default:
			return nil, fmt.Errorf("%w: unsupported entry type for %q", errInvalidArtifact, header.Name)
		}
	}
	if !hasFile {
		return nil, fmt.Errorf("%w: tar contains no files", errInvalidArtifact)
	}
	out := make([]apitypes.FirmwareArtifactEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	sortArtifactEntries(out)
	return out, nil
}

func addArtifactParentDirs(entries map[string]apitypes.FirmwareArtifactEntry, entryPath string, modTime time.Time) error {
	dir := path.Dir(entryPath)
	for dir != "." && dir != "/" && dir != "" {
		if existing, exists := entries[dir]; exists {
			if existing.Type == apitypes.FirmwareArtifactEntryTypeFile {
				return fmt.Errorf("%w: path conflict %q", errInvalidArtifact, dir)
			}
		} else {
			entries[dir] = apitypes.FirmwareArtifactEntry{
				Path:    dir,
				Type:    apitypes.FirmwareArtifactEntryTypeDir,
				Mode:    0755,
				ModTime: modTime.UTC(),
			}
		}
		dir = path.Dir(dir)
	}
	return nil
}

func addArtifactDir(entries map[string]apitypes.FirmwareArtifactEntry, entryPath string, header *tar.Header) error {
	if existing, exists := entries[entryPath]; exists && existing.Type == apitypes.FirmwareArtifactEntryTypeFile {
		return fmt.Errorf("%w: path conflict %q", errInvalidArtifact, entryPath)
	}
	if err := addArtifactParentDirs(entries, entryPath, header.ModTime); err != nil {
		return err
	}
	entries[entryPath] = apitypes.FirmwareArtifactEntry{
		Path:    entryPath,
		Type:    apitypes.FirmwareArtifactEntryTypeDir,
		Mode:    int32(header.Mode),
		ModTime: header.ModTime.UTC(),
	}
	return nil
}

func (s *Server) readArtifactManifest(slot *apitypes.FirmwareSlot) (artifactManifest, error) {
	if slot == nil || slot.Artifact == nil {
		return artifactManifest{}, errArtifactNotFound
	}
	assets, err := s.assets()
	if err != nil {
		return artifactManifest{}, err
	}
	reader, err := assets.Get(slot.Artifact.ManifestPath)
	if err != nil {
		return artifactManifest{}, errArtifactNotFound
	}
	defer reader.Close()
	var manifest artifactManifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return artifactManifest{}, err
	}
	return manifest, nil
}

func artifactListItems(entries []apitypes.FirmwareArtifactEntry, target string) ([]apitypes.FirmwareArtifactEntry, error) {
	if target != "" {
		entry, ok := findArtifactEntry(entries, target)
		if !ok {
			return nil, errArtifactNotFound
		}
		if entry.Type == apitypes.FirmwareArtifactEntryTypeFile {
			return []apitypes.FirmwareArtifactEntry{entry}, nil
		}
	}
	items := make([]apitypes.FirmwareArtifactEntry, 0)
	for _, entry := range entries {
		if parentArtifactPath(entry.Path) == target {
			items = append(items, entry)
		}
	}
	sortArtifactEntries(items)
	return items, nil
}

func artifactTreeItems(entries []apitypes.FirmwareArtifactEntry, target string) ([]apitypes.FirmwareArtifactEntry, error) {
	if target != "" {
		entry, ok := findArtifactEntry(entries, target)
		if !ok {
			return nil, errArtifactNotFound
		}
		if entry.Type != apitypes.FirmwareArtifactEntryTypeDir {
			return nil, fmt.Errorf("%w: path is not a directory", errInvalidArtifact)
		}
	}
	items := make([]apitypes.FirmwareArtifactEntry, 0, len(entries))
	for _, entry := range entries {
		if target == "" || strings.HasPrefix(entry.Path, target+"/") {
			items = append(items, entry)
		}
	}
	sortArtifactEntries(items)
	return items, nil
}

func artifactStats(artifact apitypes.FirmwareArtifact, entries []apitypes.FirmwareArtifactEntry, target string) (apitypes.FirmwareArtifactStats, error) {
	stats := apitypes.FirmwareArtifactStats{
		Artifact: artifact,
		Path:     nil,
	}
	scope := entries
	if target != "" {
		entry, ok := findArtifactEntry(entries, target)
		if !ok {
			return apitypes.FirmwareArtifactStats{}, errArtifactNotFound
		}
		stats.Path = &target
		stats.Entry = &entry
		if entry.Type == apitypes.FirmwareArtifactEntryTypeFile {
			scope = []apitypes.FirmwareArtifactEntry{entry}
		} else {
			scope = artifactDescendants(entries, target)
		}
	}
	for _, entry := range scope {
		if entry.Type == apitypes.FirmwareArtifactEntryTypeFile {
			stats.FilesCount++
			stats.TotalSize += entry.Size
		}
	}
	return stats, nil
}

func artifactDescendants(entries []apitypes.FirmwareArtifactEntry, target string) []apitypes.FirmwareArtifactEntry {
	out := make([]apitypes.FirmwareArtifactEntry, 0)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Path, target+"/") {
			out = append(out, entry)
		}
	}
	return out
}

func findArtifactEntry(entries []apitypes.FirmwareArtifactEntry, target string) (apitypes.FirmwareArtifactEntry, bool) {
	for _, entry := range entries {
		if entry.Path == target {
			return entry, true
		}
	}
	return apitypes.FirmwareArtifactEntry{}, false
}

func parentArtifactPath(entryPath string) string {
	parent := path.Dir(entryPath)
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}

func normalizeArtifactPath(raw string, allowEmpty bool) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("%w: path is required", errInvalidArtifact)
	}
	if strings.HasPrefix(raw, "/") || strings.Contains(raw, "\x00") {
		return "", fmt.Errorf("%w: unsafe path %q", errInvalidArtifact, raw)
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", fmt.Errorf("%w: unsafe path %q", errInvalidArtifact, raw)
		}
	}
	cleaned := path.Clean(raw)
	if cleaned == "." {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("%w: path is required", errInvalidArtifact)
	}
	return cleaned, nil
}

func sortArtifactEntries(entries []apitypes.FirmwareArtifactEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == apitypes.FirmwareArtifactEntryTypeDir
		}
		return entries[i].Path < entries[j].Path
	})
}

func artifactPrefixHasObjects(assets objectstore.ObjectStore, prefix string) (bool, error) {
	objects, err := assets.List(prefix)
	if err != nil {
		return false, err
	}
	return len(objects) > 0, nil
}

func firmwareArtifactPrefix(name, channel string) string {
	return path.Join(firmwareAssetPrefix(name), objectPathSegment(channel), "artifact")
}

func mergeArtifactMetadata(previous apitypes.FirmwareSlots, next *apitypes.FirmwareSlots) {
	next.Stable.Artifact = previous.Stable.Artifact
	next.Beta.Artifact = previous.Beta.Artifact
	next.Develop.Artifact = previous.Develop.Artifact
	next.Pending.Artifact = previous.Pending.Artifact
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type byteCounter struct {
	N int64
}

func (c *byteCounter) Write(p []byte) (int, error) {
	c.N += int64(len(p))
	return len(p), nil
}

func downloadArtifactError(err error) adminservice.DownloadFirmwareArtifactResponseObject {
	switch {
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errArtifactNotFound):
		return adminservice.DownloadFirmwareArtifact404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error()))
	default:
		return adminservice.DownloadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}

func uploadArtifactError(err error) adminservice.UploadFirmwareArtifactResponseObject {
	switch {
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound):
		return adminservice.UploadFirmwareArtifact404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_NOT_FOUND", err.Error()))
	case errors.Is(err, errInvalidArtifact), errors.Is(err, errInvalidChannel):
		return adminservice.UploadFirmwareArtifact400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT", err.Error()))
	default:
		return adminservice.UploadFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}

func deleteArtifactError(err error) adminservice.DeleteFirmwareArtifactResponseObject {
	switch {
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errArtifactNotFound):
		return adminservice.DeleteFirmwareArtifact404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error()))
	default:
		return adminservice.DeleteFirmwareArtifact500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}

func listArtifactError(err error) adminservice.ListFirmwareArtifactEntriesResponseObject {
	switch {
	case errors.Is(err, errInvalidArtifact):
		return adminservice.ListFirmwareArtifactEntries400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error()))
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errArtifactNotFound):
		return adminservice.ListFirmwareArtifactEntries404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error()))
	default:
		return adminservice.ListFirmwareArtifactEntries500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}

func treeArtifactError(err error) adminservice.TreeFirmwareArtifactEntriesResponseObject {
	switch {
	case errors.Is(err, errInvalidArtifact):
		return adminservice.TreeFirmwareArtifactEntries400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error()))
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errArtifactNotFound):
		return adminservice.TreeFirmwareArtifactEntries404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error()))
	default:
		return adminservice.TreeFirmwareArtifactEntries500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}

func statArtifactError(err error) adminservice.StatFirmwareArtifactEntryResponseObject {
	switch {
	case errors.Is(err, errInvalidArtifact):
		return adminservice.StatFirmwareArtifactEntry400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error()))
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errArtifactNotFound):
		return adminservice.StatFirmwareArtifactEntry404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error()))
	default:
		return adminservice.StatFirmwareArtifactEntry500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}

func downloadEntryError(err error) adminservice.DownloadFirmwareArtifactEntryResponseObject {
	switch {
	case errors.Is(err, errInvalidArtifact):
		return adminservice.DownloadFirmwareArtifactEntry400JSONResponse(apitypes.NewErrorResponse("INVALID_FIRMWARE_ARTIFACT_PATH", err.Error()))
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, errChannelNotFound), errors.Is(err, errArtifactNotFound):
		return adminservice.DownloadFirmwareArtifactEntry404JSONResponse(apitypes.NewErrorResponse("FIRMWARE_ARTIFACT_NOT_FOUND", err.Error()))
	default:
		return adminservice.DownloadFirmwareArtifactEntry500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error()))
	}
}
