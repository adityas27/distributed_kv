package persistence

import (
	"fmt"
	"sync/atomic"
	"time"
)

type ManagedCache interface {
	SnapshotEntries() []SnapshotEntry
	SetWAL(*WAL)
}

type SnapshotManager struct {
	cache          ManagedCache
	walInstance    *WAL
	snapshotWriter *SnapshotWriter
	interval       time.Duration
	ticker         *time.Ticker
	done           chan struct{}
	isRunning      int32
}

type SnapshotManagerConfig struct {
	SnapshotInterval time.Duration
	SnapshotDir      string
	SnapshotFilename string
}

func DefaultSnapshotManagerConfig() SnapshotManagerConfig {
	return SnapshotManagerConfig{
		SnapshotInterval: 5 * time.Minute,
		SnapshotDir:      ".",
		SnapshotFilename: "snapshot.json",
	}
}

func NewSnapshotManager(
	cache ManagedCache,
	walInstance *WAL,
	config SnapshotManagerConfig,
) *SnapshotManager {

	snapshotConfig := NewSnapshotConfig(
		config.SnapshotDir,
		config.SnapshotFilename,
	)

	return &SnapshotManager{
		cache:          cache,
		walInstance:    walInstance,
		snapshotWriter: NewSnapshotWriter(snapshotConfig),
		interval:       config.SnapshotInterval,
		done:           make(chan struct{}),
	}
}

func (sm *SnapshotManager) Start() error {
	if !atomic.CompareAndSwapInt32(&sm.isRunning, 0, 1) {
		return fmt.Errorf("snapshot manager already running")
	}

	sm.ticker = time.NewTicker(sm.interval)

	go func() {
		for {
			select {
			case <-sm.done:
				return

			case <-sm.ticker.C:
				if err := sm.snapshotWriter.CreateSnapshot(sm.cache); err != nil {
					continue
				}

				if _, err := sm.walInstance.Rotate(); err != nil {
					continue
				}

				newWAL, err := NewWAL(sm.walInstance.Path())
				if err != nil {
					continue
				}

				sm.walInstance = newWAL
				sm.cache.SetWAL(newWAL)
			}
		}
	}()

	return nil
}

func (sm *SnapshotManager) Stop() error {
	if !atomic.CompareAndSwapInt32(&sm.isRunning, 1, 0) {
		return fmt.Errorf("snapshot manager not running")
	}

	sm.ticker.Stop()
	close(sm.done)

	return nil
}

func (sm *SnapshotManager) CreateSnapshotNow() error {
	if err := sm.snapshotWriter.CreateSnapshot(sm.cache); err != nil {
		return err
	}

	if _, err := sm.walInstance.Rotate(); err != nil {
		return err
	}

	newWAL, err := NewWAL(sm.walInstance.Path())
	if err != nil {
		return err
	}

	sm.walInstance = newWAL
	sm.cache.SetWAL(newWAL)

	return nil
}
func (sm *SnapshotManager) IsRunning() bool {
	return atomic.LoadInt32(&sm.isRunning) == 1
}
