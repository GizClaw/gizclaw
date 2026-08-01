package localserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	// PIXACommit identifies the immutable GizClaw/pixa source used by Desktop bootstrap.
	PIXACommit = "5fed581ae87ac3cf4a5a05952d43edebbbed8d9f"
	// PIXAAssetBaseURL points at the Git LFS media endpoint for the pinned commit.
	PIXAAssetBaseURL = "https://media.githubusercontent.com/media/GizClaw/pixa/" + PIXACommit + "/assets/codex-pets/"
)

var pixaAssetNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*\.pixa$`)

type pixaResolver struct {
	cacheDir     string
	assetBaseURL string
	httpClient   *http.Client
}

func newPIXAResolver(cacheDir string) (*pixaResolver, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return nil, errors.New("pixa assets: cache directory is required")
	}
	return &pixaResolver{
		cacheDir:     cacheDir,
		assetBaseURL: PIXAAssetBaseURL,
		httpClient:   &http.Client{Timeout: 20 * time.Second},
	}, nil
}

func (r *pixaResolver) resolve(ctx context.Context, name string, expectedWidth, expectedHeight uint16) ([]byte, error) {
	if r == nil || strings.TrimSpace(r.cacheDir) == "" {
		return nil, errors.New("pixa assets: resolver is not configured")
	}
	if !pixaAssetNamePattern.MatchString(name) {
		return nil, fmt.Errorf("pixa assets: unsafe asset name %q", name)
	}
	if err := r.secureCacheDir(); err != nil {
		return nil, err
	}
	data, cacheReadErr := r.readCache(name)
	var cacheErr error
	if cacheReadErr == nil {
		if validateErr := validatePIXAData(data, name, expectedWidth, expectedHeight); validateErr == nil {
			if err := r.removeCacheBackup(name); err != nil {
				return nil, err
			}
			return data, nil
		} else {
			cacheErr = fmt.Errorf("validate cached asset: %w", validateErr)
		}
	} else if !errors.Is(cacheReadErr, os.ErrNotExist) {
		cacheErr = fmt.Errorf("read cached asset: %w", cacheReadErr)
	}
	backupData, backupReadErr := r.readCacheFile(r.cacheBackupFile(name))
	if backupReadErr == nil {
		if validateErr := validatePIXAData(backupData, name, expectedWidth, expectedHeight); validateErr == nil {
			if err := r.restoreCacheBackup(name); err != nil {
				return nil, err
			}
			return backupData, nil
		} else {
			cacheErr = errors.Join(cacheErr, fmt.Errorf("validate cached asset backup: %w", validateErr))
		}
	} else if !errors.Is(backupReadErr, os.ErrNotExist) {
		cacheErr = errors.Join(cacheErr, fmt.Errorf("read cached asset backup: %w", backupReadErr))
	}

	candidate, downloadErr := r.download(ctx, name)
	if downloadErr != nil {
		if cacheErr != nil {
			return nil, fmt.Errorf("pixa assets: load %s at %s: %w", name, PIXACommit, errors.Join(cacheErr, downloadErr))
		}
		return nil, fmt.Errorf("pixa assets: load %s at %s: %w", name, PIXACommit, downloadErr)
	}
	if err := validatePIXAData(candidate, name, expectedWidth, expectedHeight); err != nil {
		if cacheErr != nil {
			return nil, fmt.Errorf("pixa assets: validate %s at %s: %w", name, PIXACommit, errors.Join(cacheErr, err))
		}
		return nil, fmt.Errorf("pixa assets: validate %s at %s: %w", name, PIXACommit, err)
	}
	if err := r.writeCache(name, candidate); err != nil {
		return nil, err
	}
	return candidate, nil
}

func (r *pixaResolver) commitCacheDir() string {
	return filepath.Join(r.cacheDir, PIXACommit)
}

func (r *pixaResolver) cacheFile(name string) string {
	return filepath.Join(r.commitCacheDir(), name)
}

func (r *pixaResolver) cacheBackupFile(name string) string {
	return r.cacheFile(name) + ".backup"
}

func (r *pixaResolver) secureCacheDir() error {
	for _, dir := range []string{r.cacheDir, r.commitCacheDir()} {
		info, err := os.Lstat(dir)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("pixa assets: cache directory %q must not be a symbolic link", dir)
			}
			if !info.IsDir() {
				return fmt.Errorf("pixa assets: cache path %q is not a directory", dir)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("pixa assets: inspect cache directory: %w", err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("pixa assets: create cache directory: %w", err)
		}
		info, err = os.Lstat(dir)
		if err != nil {
			return fmt.Errorf("pixa assets: inspect cache directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("pixa assets: cache directory %q must be a real directory", dir)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("pixa assets: secure cache directory: %w", err)
		}
	}
	return nil
}

func (r *pixaResolver) readCache(name string) ([]byte, error) {
	return r.readCacheFile(r.cacheFile(name))
}

func (r *pixaResolver) readCacheFile(file string) ([]byte, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("pixa assets: cache entry %q must be a regular file", file)
	}
	if info.Size() <= 0 || info.Size() > maxBootstrapAssetBytes {
		return nil, fmt.Errorf("pixa assets: cache entry %q size %d is outside 1..%d", file, info.Size(), maxBootstrapAssetBytes)
	}
	return os.ReadFile(file)
}

func (r *pixaResolver) removeCacheBackup(name string) error {
	backupFile := r.cacheBackupFile(name)
	info, err := os.Lstat(backupFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("pixa assets: inspect cache backup: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("pixa assets: cache entry %q must be a regular file", backupFile)
	}
	if err := os.Remove(backupFile); err != nil {
		return fmt.Errorf("pixa assets: remove cache backup: %w", err)
	}
	return nil
}

func (r *pixaResolver) restoreCacheBackup(name string) error {
	cacheFile := r.cacheFile(name)
	if info, err := os.Lstat(cacheFile); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("pixa assets: cache entry %q must be a regular file", cacheFile)
		}
		if err := os.Remove(cacheFile); err != nil {
			return fmt.Errorf("pixa assets: remove invalid cache entry: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("pixa assets: inspect cache entry: %w", err)
	}
	if err := os.Rename(r.cacheBackupFile(name), cacheFile); err != nil {
		return fmt.Errorf("pixa assets: restore cache backup: %w", err)
	}
	return nil
}

func (r *pixaResolver) download(ctx context.Context, name string) ([]byte, error) {
	client := r.httpClient
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	baseURL := r.assetBaseURL
	if baseURL == "" {
		baseURL = PIXAAssetBaseURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+name, nil)
	if err != nil {
		return nil, fmt.Errorf("create asset request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download asset: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("download asset: HTTP %s", response.Status)
	}
	if response.ContentLength > maxBootstrapAssetBytes {
		return nil, fmt.Errorf("asset size %d exceeds %d", response.ContentLength, maxBootstrapAssetBytes)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBootstrapAssetBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read asset: %w", err)
	}
	if len(data) == 0 || len(data) > maxBootstrapAssetBytes {
		return nil, fmt.Errorf("asset size %d is outside 1..%d", len(data), maxBootstrapAssetBytes)
	}
	return data, nil
}

func (r *pixaResolver) writeCache(name string, data []byte) error {
	return r.writeCacheWithRename(name, data, os.Rename)
}

func (r *pixaResolver) writeCacheWithRename(name string, data []byte, rename func(string, string) error) error {
	if err := r.secureCacheDir(); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(r.commitCacheDir(), "."+name+"-*.tmp")
	if err != nil {
		return fmt.Errorf("pixa assets: create cache candidate: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("pixa assets: secure cache candidate: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("pixa assets: write cache candidate: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("pixa assets: sync cache candidate: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("pixa assets: close cache candidate: %w", err)
	}

	cacheFile := r.cacheFile(name)
	backupFile := r.cacheBackupFile(name)
	if err := r.removeCacheBackup(name); err != nil {
		return err
	}
	hadPrevious := false
	if info, err := os.Lstat(cacheFile); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("pixa assets: cache entry %q must be a regular file", cacheFile)
		}
		if err := rename(cacheFile, backupFile); err != nil {
			return fmt.Errorf("pixa assets: back up cache entry: %w", err)
		}
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("pixa assets: inspect cache entry: %w", err)
	}
	if err := rename(temporaryName, cacheFile); err != nil {
		activationErr := fmt.Errorf("pixa assets: activate cache candidate: %w", err)
		if hadPrevious {
			if restoreErr := rename(backupFile, cacheFile); restoreErr != nil {
				return errors.Join(activationErr, fmt.Errorf("pixa assets: restore previous cache entry from %q: %w", backupFile, restoreErr))
			}
		}
		return activationErr
	}
	if hadPrevious {
		if err := os.Remove(backupFile); err != nil {
			return fmt.Errorf("pixa assets: remove previous cache backup: %w", err)
		}
	}
	return nil
}
