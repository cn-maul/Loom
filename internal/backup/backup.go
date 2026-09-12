// Package backup gives Loom's local SQLite store a copyable, verifiable
// lifecycle: scheduled snapshots, on-demand export and pre-restore validation.
//
// Snapshots use `VACUUM INTO`, which produces a consistent, compacted copy of
// the database even while WAL is active, so a backup never needs the server to
// pause. Restore is deliberately two-phase: the chosen backup is validated and
// staged next to the live database, and the swap happens at the next startup
// before any connection is opened. Swapping under a running process would leave
// every repository holding a dead connection, and half-applied restores are
// worse than a restart.
package backup

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const filePrefix = "loom-backup-"
const fileSuffix = ".db"

// Info describes one snapshot on disk.
type Info struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	ModifiedAt time.Time `json:"modified_at"`
	Encrypted  bool      `json:"encrypted"`
}

// Status is the observable state of the backup scheduler. The frontend reads
// it so a failing backup is a visible fact instead of a silent gap.
type Status struct {
	Enabled     bool      `json:"enabled"`
	Dir         string    `json:"dir"`
	IntervalH   int       `json:"interval_hours"`
	Keep        int       `json:"keep"`
	LastRun     time.Time `json:"last_run"`
	LastError   string    `json:"last_error"`
	NextRun     time.Time `json:"next_run"`
	BackupCount int       `json:"backup_count"`
}

// Service owns the backup directory, the schedule and the snapshot routine.
type Service struct {
	db         *sql.DB
	dbPath     string
	dir        string
	interval   time.Duration
	keep       int
	passphrase string

	mu      sync.Mutex
	lastRun time.Time
	lastErr string
	nextRun time.Time

	stopCh      chan struct{}
	stopped     chan struct{}
	stopOnce    sync.Once
	stoppedOnce sync.Once
	running     atomic.Bool // true while the scheduler goroutine is alive
}

// NewService wires the backup service. keep <= 0 disables pruning; interval
// <= 0 disables the schedule (manual backups still work). A non-empty
// passphrase encrypts every new snapshot with AES-256-GCM.
func NewService(db *sql.DB, dbPath, dir string, interval time.Duration, keep int, passphrase string) *Service {
	return &Service{
		db:         db,
		dbPath:     dbPath,
		dir:        dir,
		interval:   interval,
		keep:       keep,
		passphrase: passphrase,
		stopCh:     make(chan struct{}),
		stopped:    make(chan struct{}),
	}
}

// Create takes one snapshot: VACUUM INTO a temporary file, then rename to the
// final timestamped name. The rename stays inside the backup directory, so it
// is atomic on the same volume — a crash mid-backup leaves at most an orphaned
// temp file, never a half-written snapshot that looks complete.
func (s *Service) Create() (Info, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		s.record(time.Now(), fmt.Sprintf("create backup dir: %v", err))
		return Info{}, fmt.Errorf("create backup dir: %w", err)
	}

	tmp := filepath.Join(s.dir, fmt.Sprintf(".vacuum-%d.tmp", time.Now().UnixNano()))
	// VACUUM INTO refuses to overwrite; the deferred remove cleans up both the
	// failure path and the success path (after the rename below).
	if _, err := s.db.Exec(`VACUUM INTO ?`, tmp); err != nil {
		os.Remove(tmp)
		s.record(time.Now(), fmt.Sprintf("vacuum into: %v", err))
		return Info{}, fmt.Errorf("vacuum into snapshot: %w", err)
	}

	name := filePrefix + time.Now().UTC().Format("20060102-150405") + fileSuffix
	dst := filepath.Join(s.dir, name)
	if _, err := os.Stat(dst); err == nil {
		// Two snapshots within the same second: keep both by suffixing.
		name = filePrefix + time.Now().UTC().Format("20060102-150405") + fmt.Sprintf("-%d", time.Now().UnixNano()%1000) + fileSuffix
		dst = filepath.Join(s.dir, name)
	}
	if s.passphrase != "" {
		// Encrypted mode: the VACUUM output is sealed into a .db.enc file and
		// the plaintext intermediate is destroyed immediately.
		dst += ".enc"
		name += ".enc"
		if err := encryptFile(tmp, dst, s.passphrase); err != nil {
			os.Remove(tmp)
			s.record(time.Now(), fmt.Sprintf("encrypt snapshot: %v", err))
			return Info{}, fmt.Errorf("encrypt snapshot: %w", err)
		}
		os.Remove(tmp)
	} else if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		s.record(time.Now(), fmt.Sprintf("rename snapshot: %v", err))
		return Info{}, fmt.Errorf("finalize snapshot: %w", err)
	}

	pruned := 0
	if s.keep > 0 {
		var err error
		pruned, err = s.prune()
		if err != nil {
			// The snapshot itself succeeded; a failed prune must not fail the call.
			log.Printf("WARNING: backup prune failed: %v", err)
		}
	}
	log.Printf("backup created: %s (pruned %d old)", name, pruned)

	stat, err := os.Stat(dst)
	mod := time.Now()
	if err == nil {
		mod = stat.ModTime()
	}
	size := int64(0)
	if err == nil {
		size = stat.Size()
	}
	s.record(time.Now(), "")
	return Info{Name: name, SizeBytes: size, ModifiedAt: mod, Encrypted: s.passphrase != ""}, nil
}

// List returns the snapshots in the backup directory, newest first.
func (s *Service) List() ([]Info, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, filePrefix) {
			continue
		}
		if !strings.HasSuffix(name, fileSuffix) && !strings.HasSuffix(name, fileSuffix+".enc") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, Info{
			Name:       name,
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
			Encrypted:  strings.HasSuffix(name, ".enc"),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out, nil
}

// prune deletes the oldest snapshots beyond the retention count. It relies on
// the timestamped names sorting lexically into chronological order.
func (s *Service) prune() (int, error) {
	all, err := s.List()
	if err != nil {
		return 0, err
	}
	if len(all) <= s.keep {
		return 0, nil
	}
	removed := 0
	for _, b := range all[s.keep:] {
		if err := os.Remove(filepath.Join(s.dir, b.Name)); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

// ResolvePath maps a user-supplied backup reference to a file. A bare name is
// looked up inside the backup directory; an absolute path is used as-is, so a
// snapshot copied elsewhere can still be restored.
func (s *Service) ResolvePath(name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(s.dir, filepath.Clean(name))
}

// Start launches the schedule. The first snapshot runs shortly after startup,
// so a machine that reboots daily still gets one backup per day. Safe to call
// twice; a no-op when the interval is disabled.
func (s *Service) Start() {
	if s.interval <= 0 {
		return
	}
	// A closed stopCh means Stop already ran: starting now would exit
	// immediately and close the already-closed stopped channel.
	select {
	case <-s.stopCh:
		return
	default:
	}
	if !s.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer close(s.stopped)
		defer s.running.Store(false)
		const firstDelay = 15 * time.Second
		s.mu.Lock()
		s.nextRun = time.Now().Add(firstDelay)
		s.mu.Unlock()
		timer := time.NewTimer(firstDelay)
		defer timer.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-timer.C:
			}
			if _, err := s.Create(); err != nil {
				log.Printf("WARNING: scheduled backup failed: %v", err)
			}
			s.mu.Lock()
			s.nextRun = time.Now().Add(s.interval)
			s.mu.Unlock()
			timer.Reset(s.interval)
		}
	}()
}

// Stop terminates the schedule and waits for the goroutine to exit. Safe to
// call when the schedule was never started — an App with backups disabled
// must still shut down cleanly. An in-flight snapshot is not interrupted:
// VACUUM INTO is fast and aborting it would only leave an orphaned temp file
// for the next Create to ignore.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
	// When the goroutine is alive it owns closing stopped; otherwise Stop
	// itself must, or the wait below would block forever.
	if !s.running.Load() {
		s.stoppedOnce.Do(func() { close(s.stopped) })
	}
	<-s.stopped
}

func (s *Service) record(at time.Time, errMsg string) {
	s.mu.Lock()
	s.lastRun = at
	s.lastErr = errMsg
	s.mu.Unlock()
}

// ExportJSON serializes the live database to a JSON document.
func (s *Service) ExportJSON(redacted bool) ([]byte, error) {
	return Export(s.db, redacted)
}

// Status reports the scheduler state for the API.
func (s *Service) Status() Status {
	list, _ := s.List()
	s.mu.Lock()
	defer s.mu.Unlock()
	var intervalH int
	if s.interval > 0 {
		intervalH = int(s.interval.Hours())
	}
	return Status{
		Enabled:     s.interval > 0,
		Dir:         s.dir,
		IntervalH:   intervalH,
		Keep:        s.keep,
		LastRun:     s.lastRun,
		LastError:   s.lastErr,
		NextRun:     s.nextRun,
		BackupCount: len(list),
	}
}
