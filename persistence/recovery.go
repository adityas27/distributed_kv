package persistence

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RecoveryManager struct {
	config  SnapshotConfig
	walPath string
}

func NewRecoveryManager(config SnapshotConfig, walPath string) *RecoveryManager {
	return &RecoveryManager{
		config:  config,
		walPath: walPath,
	}
}

type RecoveryStats struct {
	SnapshotEntriesRestored int
	WALEntriesReplayed      int
	ExpiredEntriesSkipped   int
	RecoveryTime            time.Duration
}

type RecoveryCache interface {
	RestoreEntry(key, value string, ttl int, expiresAt time.Time)
	RestoreDelete(key string)
}

func (rm *RecoveryManager) Recover(cache RecoveryCache) (*RecoveryStats, error) {
	start := time.Now()

	stats := &RecoveryStats{}

	path := filepath.Join(rm.config.Directory, rm.config.Filename)

	var snapshot Snapshot

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, err
		}

		now := time.Now()

		for _, entry := range snapshot.Entries {
			if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
				stats.ExpiredEntriesSkipped++
				continue
			}

			ttl := 0
			if !entry.ExpiresAt.IsZero() {
				ttl = int(entry.ExpiresAt.Sub(now).Seconds())
			}

			cache.RestoreEntry(
				entry.Key,
				entry.Value,
				ttl,
				entry.ExpiresAt,
			)

			stats.SnapshotEntriesRestored++
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	walStats, err := rm.replayWAL(cache, snapshot.CreatedAt)
	if err != nil {
		return nil, err
	}

	stats.WALEntriesReplayed = walStats.EntriesReplayed
	stats.ExpiredEntriesSkipped += walStats.ExpiredEntriesSkipped
	stats.RecoveryTime = time.Since(start)

	return stats, nil
}

type WALReplayStats struct {
	EntriesReplayed       int
	ExpiredEntriesSkipped int
}

func (rm *RecoveryManager) replayWAL(cache RecoveryCache, snapshotTime time.Time) (*WALReplayStats, error) {
	stats := &WALReplayStats{}

	files, err := rm.walFiles()
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}

		return stats, err
	}

	now := time.Now()

	for _, path := range files {
		replayed, expired, err := replayWALFile(path, cache, snapshotTime, now)
		if err != nil {
			return nil, err
		}

		stats.EntriesReplayed += replayed
		stats.ExpiredEntriesSkipped += expired
	}

	return stats, nil
}

func (rm *RecoveryManager) walFiles() ([]string, error) {
	dir := filepath.Dir(rm.walPath)
	base := filepath.Base(rm.walPath)
	prefix := strings.TrimSuffix(base, filepath.Ext(base)) + "-"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	type walFile struct {
		path string
		mod  time.Time
	}

	var files []walFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if name != base && !strings.HasPrefix(name, prefix) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		files = append(files, walFile{
			path: filepath.Join(dir, name),
			mod:  info.ModTime(),
		})
	}

	if len(files) == 0 {
		return nil, os.ErrNotExist
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].mod.Equal(files[j].mod) {
			return files[i].path < files[j].path
		}

		return files[i].mod.Before(files[j].mod)
	})

	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}

	return paths, nil
}

func replayWALFile(path string, cache RecoveryCache, snapshotTime, now time.Time) (int, int, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	replayed := 0
	skipped := 0

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if err == io.EOF {
				break
			}

			return replayed, skipped, err
		}

		line = bytesTrimSpace(line)
		if len(line) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}

		var entry WALEntry
		if unmarshalErr := json.Unmarshal(line, &entry); unmarshalErr != nil {
			log.Printf("skipping corrupt WAL entry in %s: %v", path, unmarshalErr)
			if err == io.EOF {
				break
			}
			continue
		}

		entryTime := time.Unix(entry.Timestamp, 0)
		if !snapshotTime.IsZero() && entryTime.Before(snapshotTime) {
			if err == io.EOF {
				break
			}
			continue
		}

		switch entry.Op {
		case SET:
			expiresAt := time.Time{}

			if entry.TTL > 0 {
				expiresAt = entryTime.Add(time.Duration(entry.TTL) * time.Second)
				if now.After(expiresAt) {
					skipped++
					if err == io.EOF {
						break
					}
					continue
				}
			}

			ttl := 0
			if !expiresAt.IsZero() {
				ttl = int(expiresAt.Sub(now).Seconds())
			}

			cache.RestoreEntry(entry.Key, entry.Value, ttl, expiresAt)
			replayed++

		case DELETE:
			cache.RestoreDelete(entry.Key)
			replayed++
		}

		if err == io.EOF {
			break
		}
	}

	return replayed, skipped, nil
}

func bytesTrimSpace(data []byte) []byte {
	start := 0
	end := len(data)

	for start < end {
		switch data[start] {
		case ' ', '\t', '\n', '\r':
			start++
		default:
			goto tail
		}
	}

tail:
	for end > start {
		switch data[end-1] {
		case ' ', '\t', '\n', '\r':
			end--
		default:
			return data[start:end]
		}
	}

	return data[start:end]
}

func (rm *RecoveryManager) RotateWAL(w *WAL) error {
	_, err := w.Rotate()
	return err
}

func (rm *RecoveryManager) CreateNewWALFile() (*WAL, error) {
	return NewWAL(rm.walPath)
}
