package objectstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync/atomic"
	"time"
)

// Root is an ObjectStore that borrows an os.Root. It never closes the root.
type Root struct {
	root *os.Root
}

var rootTempSequence atomic.Uint64

// NewRoot creates an ObjectStore over an already-open rooted filesystem.
func NewRoot(root *os.Root) (*Root, error) {
	if root == nil {
		return nil, errors.New("objectstore: filesystem root is nil")
	}
	return &Root{root: root}, nil
}

func (r *Root) Get(name string) (io.ReadCloser, error) {
	name, err := cleanName(name, false)
	if err != nil {
		return nil, err
	}
	if expired, err := r.expired(name, time.Now()); err != nil {
		return nil, err
	} else if expired {
		_ = r.Delete(name)
		return nil, fs.ErrNotExist
	}
	return r.root.Open(name)
}

func (r *Root) Put(name string, reader io.Reader) error {
	return r.put(name, reader, time.Time{})
}

func (r *Root) PutWithDeadline(name string, reader io.Reader, deadline time.Time) error {
	return r.put(name, reader, deadline)
}

func (r *Root) PutWithTTL(name string, reader io.Reader, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("objectstore: ttl must be positive")
	}
	return r.put(name, reader, time.Now().Add(ttl))
}

func (r *Root) put(name string, reader io.Reader, deadline time.Time) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	if !deadline.IsZero() && !deadline.After(time.Now()) {
		return errors.New("objectstore: deadline must be in the future")
	}
	if err := r.root.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	if err := r.root.MkdirAll(metadataRoot+"/put", 0o755); err != nil {
		return err
	}
	tmpName := rootTempName(putTempPrefix)
	tmp, err := r.root.OpenFile(tmpName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer r.root.Remove(tmpName)
	if _, err := io.Copy(tmp, reader); err != nil {
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
	backupName := rootTempName("backup-")
	hadOld := false
	if info, statErr := r.root.Lstat(name); statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("objectstore: object path %q is not a regular file", name)
		}
		if err := r.root.Rename(name, backupName); err != nil {
			return err
		}
		hadOld = true
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return statErr
	}
	defer r.root.Remove(backupName)
	if err := r.root.Rename(tmpName, name); err != nil {
		if hadOld {
			return errors.Join(err, r.root.Rename(backupName, name))
		}
		return err
	}
	if err := r.writeMetadata(name, deadline); err != nil {
		rollbackErr := r.root.Remove(name)
		if errors.Is(rollbackErr, fs.ErrNotExist) {
			rollbackErr = nil
		}
		if hadOld {
			rollbackErr = errors.Join(rollbackErr, r.root.Rename(backupName, name))
		}
		return errors.Join(err, rollbackErr)
	}
	return nil
}

func (r *Root) Delete(name string) error {
	name, err := cleanName(name, false)
	if err != nil {
		return err
	}
	err = r.root.Remove(name)
	if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	return errors.Join(err, r.deleteMetadata(name))
}

func (r *Root) DeletePrefix(prefix string) error {
	prefix, err := cleanName(prefix, true)
	if err != nil || prefix == "" {
		return err
	}
	err = r.root.RemoveAll(prefix)
	return errors.Join(err, r.deleteMetadataPrefix(prefix))
}

func (r *Root) List(prefix string) ([]ObjectInfo, error) {
	prefix, err := cleanName(prefix, true)
	if err != nil {
		return nil, err
	}
	walkRoot := prefix
	if walkRoot == "" {
		walkRoot = "."
	}
	var out []ObjectInfo
	now := time.Now()
	err = fs.WalkDir(r.root.FS(), walkRoot, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.IsDir() {
			if name == metadataRoot || strings.HasPrefix(name, metadataRoot+"/") {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		deadline, expired, err := r.deadline(name, now)
		if err != nil {
			return err
		}
		if expired {
			_ = r.Delete(name)
			return nil
		}
		out = append(out, ObjectInfo{Name: name, Size: info.Size(), Deadline: deadline})
		return nil
	})
	return out, err
}

func (r *Root) LocalDir() (string, bool) { return r.root.Name(), true }

func (r *Root) metadataPath(name string) string {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(name))
	return metadataRoot + "/expires/" + encoded + ".json"
}

func (r *Root) writeMetadata(name string, deadline time.Time) error {
	metaName := r.metadataPath(name)
	if deadline.IsZero() {
		err := r.root.Remove(metaName)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := r.root.MkdirAll(path.Dir(metaName), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(objectMetadata{Name: name, Deadline: deadline.UTC()})
	if err != nil {
		return err
	}
	tmpName := rootTempName("metadata-")
	if err := r.root.WriteFile(tmpName, data, 0o600); err != nil {
		return err
	}
	defer r.root.Remove(tmpName)
	return r.root.Rename(tmpName, metaName)
}

func (r *Root) deleteMetadata(name string) error {
	err := r.root.Remove(r.metadataPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (r *Root) deleteMetadataPrefix(prefix string) error {
	return fs.WalkDir(r.root.FS(), metadataRoot+"/expires", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		meta, err := r.readMetadata(name)
		if err != nil {
			return err
		}
		if meta.Name == prefix || strings.HasPrefix(meta.Name, prefix+"/") {
			return r.root.Remove(name)
		}
		return nil
	})
}

func (r *Root) expired(name string, now time.Time) (bool, error) {
	_, expired, err := r.deadline(name, now)
	return expired, err
}

func (r *Root) deadline(name string, now time.Time) (time.Time, bool, error) {
	meta, err := r.readMetadata(r.metadataPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if meta.Name != name {
		return time.Time{}, false, fmt.Errorf("objectstore: metadata name mismatch for %q", name)
	}
	return meta.Deadline, !meta.Deadline.IsZero() && !now.Before(meta.Deadline), nil
}

func (r *Root) readMetadata(name string) (objectMetadata, error) {
	data, err := r.root.ReadFile(name)
	if err != nil {
		return objectMetadata{}, err
	}
	var meta objectMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return objectMetadata{}, err
	}
	return meta, nil
}

var _ ObjectStore = (*Root)(nil)
var _ LocalDirProvider = (*Root)(nil)

func rootTempName(prefix string) string {
	return fmt.Sprintf("%s/put/%s%d-%d", metadataRoot, prefix, os.Getpid(), rootTempSequence.Add(1))
}
