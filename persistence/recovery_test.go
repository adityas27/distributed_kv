package persistence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type recoveryCacheStub struct {
	entries map[string]string
}

func (c *recoveryCacheStub) RestoreEntry(key, value string, ttl int, expiresAt time.Time) {
	if c.entries == nil {
		c.entries = make(map[string]string)
	}

	c.entries[key] = value
}

func (c *recoveryCacheStub) RestoreDelete(key string) {
	delete(c.entries, key)
}

func writeSnapshotFile(t *testing.T, path string, snapshot Snapshot) {
	t.Helper()

	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeWALFile(t *testing.T, path string, entries ...WALEntry) {
	t.Helper()

	data := make([]byte, 0)
	for _, entry := range entries {
		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}

		data = append(data, line...)
		data = append(data, '\n')
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverReplaysMultipleWalFiles(t *testing.T) {
	dir := t.TempDir()
	snapshotTime := time.Now().Add(-time.Minute)

	writeSnapshotFile(t, filepath.Join(dir, "snapshot.json"), Snapshot{
		CreatedAt: snapshotTime,
		Entries: []SnapshotEntry{
			{Key: "snapshot", Value: "base"},
		},
	})

	writeWALFile(t, filepath.Join(dir, "wal-1.log"),
		WALEntry{Op: SET, Key: "archive", Value: "from-archive", Timestamp: snapshotTime.Add(time.Second).Unix()},
		WALEntry{Op: DELETE, Key: "archive", Timestamp: snapshotTime.Add(2 * time.Second).Unix()},
	)
	writeWALFile(t, filepath.Join(dir, "wal.log"),
		WALEntry{Op: SET, Key: "live", Value: "from-live", Timestamp: snapshotTime.Add(3 * time.Second).Unix()},
	)

	cache := &recoveryCacheStub{}
	rm := NewRecoveryManager(NewSnapshotConfig(dir, "snapshot.json"), filepath.Join(dir, "wal.log"))

	stats, err := rm.Recover(cache)
	if err != nil {
		t.Fatal(err)
	}

	if stats.SnapshotEntriesRestored != 1 {
		t.Fatalf("expected 1 snapshot entry, got %d", stats.SnapshotEntriesRestored)
	}

	if stats.WALEntriesReplayed != 3 {
		t.Fatalf("expected 3 WAL entries replayed, got %d", stats.WALEntriesReplayed)
	}

	if cache.entries["snapshot"] != "base" {
		t.Fatal("snapshot entry missing after recovery")
	}

	if _, ok := cache.entries["archive"]; ok {
		t.Fatal("deleted archive entry should not remain")
	}

	if cache.entries["live"] != "from-live" {
		t.Fatal("live WAL entry missing after recovery")
	}
}

func TestRecoverSkipsCorruptWalLine(t *testing.T) {
	dir := t.TempDir()
	snapshotTime := time.Now().Add(-time.Minute)

	writeSnapshotFile(t, filepath.Join(dir, "snapshot.json"), Snapshot{CreatedAt: snapshotTime})
	if err := os.WriteFile(filepath.Join(dir, "wal.log"), []byte("not json\n"), 0644); err != nil {
		t.Fatal(err)
	}
	writeWALFile(t, filepath.Join(dir, "wal-1.log"),
		WALEntry{Op: SET, Key: "ok", Value: "value", Timestamp: snapshotTime.Add(time.Second).Unix()},
	)

	cache := &recoveryCacheStub{}
	rm := NewRecoveryManager(NewSnapshotConfig(dir, "snapshot.json"), filepath.Join(dir, "wal.log"))

	stats, err := rm.Recover(cache)
	if err != nil {
		t.Fatal(err)
	}

	if stats.WALEntriesReplayed != 1 {
		t.Fatalf("expected 1 replayed entry, got %d", stats.WALEntriesReplayed)
	}

	if cache.entries["ok"] != "value" {
		t.Fatal("valid WAL entry was not replayed")
	}
}

func TestWALRotateArchivesCurrentFile(t *testing.T) {
	dir := t.TempDir()
	walPath := filepath.Join(dir, "wal.log")

	wal, err := NewWAL(walPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := wal.SetWAL("key", "value", 0); err != nil {
		t.Fatal(err)
	}

	archivePath, err := wal.Rotate()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected archived WAL file: %v", err)
	}

	newWAL, err := NewWAL(walPath)
	if err != nil {
		t.Fatal(err)
	}
	defer newWAL.Close()

	if err := newWAL.SetWAL("next", "value", 0); err != nil {
		t.Fatal(err)
	}
}
