package document

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	textencoding "github.com/itishrishikesh/lumeleaf/internal/encoding"
)

var (
	ErrDiskConflict = errors.New("file changed on disk since it was opened")
	ErrSymlinkSave  = errors.New("refusing to atomically replace a symlink")
	ErrUnstableFile = errors.New("file changed while it was being opened")
)

// Open reads and decodes a normal editable file. Large-file selection belongs
// to the opener, which can inspect the file size before choosing this path.
func Open(path string) (*Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if before.Size() != after.Size() || before.ModTime() != after.ModTime() {
		return nil, ErrUnstableFile
	}
	decoded, provenance, err := textencoding.Decode(b)
	if err != nil {
		return nil, err
	}
	d, err := NewDecoded(decoded, provenance)
	if err != nil {
		return nil, err
	}
	d.disk = fingerprintBytes(after, b)
	return d, nil
}

// Save atomically replaces path only when its current content identity matches
// the last identity recorded by Open/Save. Set Force only after an intentional
// overwrite choice in the UI. Saving through a symlink is rejected because a
// rename would replace the link itself instead of its target.
func (d *Document) Save(path string, force bool) error {
	d.mu.RLock()
	text := d.rope.Bytes()
	provenance := d.meta.Encoding
	expected := d.disk
	d.mu.RUnlock()
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return ErrSymlinkSave
	}
	if !force && expected.Valid {
		current, err := fingerprint(path)
		if err != nil {
			return err
		}
		if current != expected {
			return ErrDiskConflict
		}
	}
	encoded, err := textencoding.Encode(text, provenance)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".lumeleaf-save-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(encoded)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpName, path); err != nil {
		return err
	}
	// fsync the containing directory where supported, so a completed Save has
	// the strongest practical crash-persistence guarantee on POSIX systems.
	if runtime.GOOS != "windows" {
		if dirFile, e := os.Open(dir); e == nil {
			_ = dirFile.Sync()
			_ = dirFile.Close()
		}
	}
	f, err := fingerprint(path)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.disk = f
	d.mu.Unlock()
	return nil
}

func fingerprint(path string) (Fingerprint, error) {
	f, err := os.Open(path)
	if err != nil {
		return Fingerprint{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return Fingerprint{}, err
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return Fingerprint{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return Fingerprint{Size: info.Size(), ModTime: info.ModTime().UnixNano(), Hash: sum, Valid: true}, nil
}

func fingerprintBytes(info os.FileInfo, data []byte) Fingerprint {
	return Fingerprint{Size: info.Size(), ModTime: info.ModTime().UnixNano(), Hash: sha256.Sum256(data), Valid: true}
}
