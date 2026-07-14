package objectstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dir stores objects on the local filesystem rooted at the given directory.
//
// Object names are slash-separated keys. Directories are implementation detail;
// callers should treat this as object storage, not as a general filesystem.
type Dir string

var _ ObjectStore = Dir("")

const (
	metadataRoot  = ".objectstore-meta"
	putTempPrefix = ".objectstore-put-"
)

type objectMetadata struct {
	Name     string    `json:"name"`
	Deadline time.Time `json:"deadline"`
}

func (d Dir) Get(name string) (io.ReadCloser, error) {
	name, full, err := d.absName(name, false)
	if err != nil {
		return nil, err
	}
	if expired, err := d.expired(name, time.Now()); err != nil {
		return nil, err
	} else if expired {
		_ = d.Delete(name)
		return nil, fs.ErrNotExist
	}
	return os.Open(full)
}

func (d Dir) Put(name string, r io.Reader) error {
	return d.put(name, r, time.Time{})
}

func (d Dir) PutWithDeadline(name string, r io.Reader, deadline time.Time) error {
	return d.put(name, r, deadline)
}

func (d Dir) PutWithTTL(name string, r io.Reader, ttl time.Duration) error {
	if ttl <= 0 {
		return fmt.Errorf("objectstore: ttl must be positive")
	}
	return d.put(name, r, time.Now().Add(ttl))
}

func (d Dir) put(name string, r io.Reader, deadline time.Time) error {
	name, full, err := d.absName(name, false)
	if err != nil {
		return err
	}
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		return fmt.Errorf("objectstore: deadline must be in the future")
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(full), putTempPrefix+"*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	backupName := tmpName + ".backup"
	hadOld := false
	if err := os.Link(full, backupName); err == nil {
		hadOld = true
	} else if !os.IsNotExist(err) {
		return err
	}
	defer func() {
		_ = os.Remove(backupName)
	}()
	if err := os.Rename(tmpName, full); err != nil {
		return err
	}
	if err := d.writeMetadata(name, deadline); err != nil {
		rollbackErr := os.Remove(full)
		if os.IsNotExist(rollbackErr) {
			rollbackErr = nil
		}
		if hadOld {
			if restoreErr := os.Rename(backupName, full); restoreErr != nil {
				rollbackErr = errors.Join(rollbackErr, restoreErr)
			}
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}

func (d Dir) Delete(name string) error {
	name, full, err := d.absName(name, false)
	if err != nil {
		return err
	}
	err = os.Remove(full)
	if os.IsNotExist(err) {
		err = nil
	}
	if metaErr := d.deleteMetadata(name); err == nil {
		err = metaErr
	}
	return err
}

func (d Dir) DeletePrefix(prefix string) error {
	prefix, err := cleanName(prefix, true)
	if err != nil {
		return err
	}
	if prefix == "" {
		return nil
	}
	full := d.join(prefix)
	err = os.RemoveAll(full)
	if os.IsNotExist(err) {
		err = nil
	}
	if metaErr := d.deleteMetadataPrefix(prefix); err == nil {
		err = metaErr
	}
	return err
}

func (d Dir) List(prefix string) ([]ObjectInfo, error) {
	prefix, err := cleanName(prefix, true)
	if err != nil {
		return nil, err
	}
	root := d.join(prefix)
	var out []ObjectInfo
	now := time.Now()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			if path == d.metadataRoot() {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(entry.Name(), putTempPrefix) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(d.root(), path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		deadline, expired, err := d.deadline(name, now)
		if err != nil {
			return err
		}
		if expired {
			_ = d.Delete(name)
			return nil
		}
		out = append(out, ObjectInfo{Name: name, Size: info.Size(), Deadline: deadline})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	return out, err
}

func (d Dir) LocalDir() (string, bool) {
	return d.root(), true
}

func (d Dir) abs(name string, allowEmpty bool) (string, error) {
	_, full, err := d.absName(name, allowEmpty)
	return full, err
}

func (d Dir) absName(name string, allowEmpty bool) (string, string, error) {
	name, err := cleanName(name, allowEmpty)
	if err != nil {
		return "", "", err
	}
	return name, d.join(name), nil
}

func (d Dir) join(name string) string {
	if name == "" {
		return d.root()
	}
	return filepath.Join(d.root(), filepath.FromSlash(name))
}

func (d Dir) root() string {
	if d == "" {
		return "."
	}
	return string(d)
}

func (d Dir) metadataRoot() string {
	return filepath.Join(d.root(), metadataRoot)
}

func (d Dir) metadataPath(name string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(name))
	return filepath.Join(d.metadataRoot(), "expires", encoded+".json")
}

func (d Dir) writeMetadata(name string, deadline time.Time) error {
	path := d.metadataPath(name)
	if deadline.IsZero() {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(objectMetadata{Name: name, Deadline: deadline.UTC()})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".metadata-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (d Dir) deleteMetadata(name string) error {
	err := os.Remove(d.metadataPath(name))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (d Dir) deleteMetadataPrefix(prefix string) error {
	root := filepath.Join(d.metadataRoot(), "expires")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		meta, err := readMetadataFile(path)
		if err != nil {
			return err
		}
		if meta.Name == prefix || strings.HasPrefix(meta.Name, prefix+"/") {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (d Dir) expired(name string, now time.Time) (bool, error) {
	_, expired, err := d.deadline(name, now)
	return expired, err
}

func (d Dir) deadline(name string, now time.Time) (time.Time, bool, error) {
	meta, err := readMetadataFile(d.metadataPath(name))
	if os.IsNotExist(err) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if meta.Name != name {
		return time.Time{}, false, fmt.Errorf("objectstore: metadata name mismatch for %q", name)
	}
	if meta.Deadline.IsZero() {
		return time.Time{}, false, nil
	}
	return meta.Deadline, !now.Before(meta.Deadline), nil
}

func readMetadataFile(path string) (objectMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return objectMetadata{}, err
	}
	var meta objectMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return objectMetadata{}, err
	}
	return meta, nil
}

func cleanName(name string, allowEmpty bool) (string, error) {
	if name == "" {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("objectstore: object name is empty")
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("objectstore: invalid absolute object name %q", name)
	}

	parts := strings.Split(filepath.ToSlash(name), "/")
	out := parts[:0]
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return "", fmt.Errorf("objectstore: invalid object name %q", name)
		default:
			if strings.HasPrefix(part, putTempPrefix) {
				return "", fmt.Errorf("objectstore: reserved object name %q", name)
			}
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		if allowEmpty {
			return "", nil
		}
		return "", fmt.Errorf("objectstore: object name is empty")
	}
	name = strings.Join(out, "/")
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("objectstore: invalid object name %q", name)
	}
	if filepath.IsAbs(filepath.FromSlash(name)) {
		return "", fmt.Errorf("objectstore: invalid absolute object name %q", name)
	}
	if name == metadataRoot || strings.HasPrefix(name, metadataRoot+"/") {
		return "", fmt.Errorf("objectstore: reserved object name %q", name)
	}
	return name, nil
}
