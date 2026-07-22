package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Operation string

const (
	SET    Operation = "SET"
	DELETE Operation = "DELETE"
)

type WALEntry struct {
	Op        Operation `json:"op"`
	Key       string    `json:"key"`
	Value     string    `json:"value,omitempty"`
	TTL       int64     `json:"ttl,omitempty"` // seconds
	Timestamp int64     `json:"ts"`            // unix timestamp
}

type WAL struct {
	path string
	file *os.File
	mu   sync.Mutex
}

// NewWAL opens or creates the log file used to persist cache mutations.
// The returned handle is append-only and is safe for concurrent callers.
func NewWAL(path string) (*WAL, error) {
	f, err := os.OpenFile(
		path,
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		0644,
	)
	if err != nil {
		return nil, err
	}

	return &WAL{path: path, file: f}, nil
}

func (w *WAL) Path() string {
	return w.path
}

// SetWAL records a write operation in the log before the cache mutates memory.
// The entry is newline-delimited JSON so recovery can replay it deterministically.
func (w *WAL) SetWAL(key, value string, ttl int64) error {
	entry := WALEntry{
		Op:        SET,
		Key:       key,
		Value:     value,
		TTL:       ttl,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	_, err = w.file.Write(append(data, '\n'))
	if err != nil {
		return err
	}

	return w.file.Sync()
}

// DeleteWAL records a delete operation in the same replayable WAL format.
// Recovery uses it to remove keys that were deleted after the last snapshot.
func (w *WAL) DeleteWAL(key string) error {
	entry := WALEntry{
		Op:        DELETE,
		Key:       key,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	_, err = w.file.Write(append(data, '\n'))

	if err != nil {
		return err
	}

	return w.file.Sync()
}

// Close flushes and closes the underlying WAL file descriptor.
// Callers should close the WAL during shutdown or log rotation.
func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.file.Close()
}

func (w *WAL) Rotate() (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.file.Close(); err != nil {
		return "", err
	}

	archivePath := archivedWALPath(w.path)
	if err := os.Rename(w.path, archivePath); err != nil {
		if reopened, openErr := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644); openErr == nil {
			w.file = reopened
		}

		return "", err
	}

	_ = cleanupOldArchives(w.path, archivePath)

	return archivePath, nil
}

func archivedWALPath(activePath string) string {
	dir := filepath.Dir(activePath)
	base := filepath.Base(activePath)
	name := fmt.Sprintf("%s-%d.log", base[:len(base)-len(filepath.Ext(base))], time.Now().UnixNano())
	return filepath.Join(dir, name)
}

func cleanupOldArchives(activePath, keepPath string) error {
	dir := filepath.Dir(activePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		if path == activePath || path == keepPath {
			continue
		}

		if filepath.Ext(entry.Name()) != ".log" {
			continue
		}

		if len(entry.Name()) < 6 || entry.Name()[:4] != "wal-" {
			continue
		}

		_ = os.Remove(path)
	}

	return nil
}
