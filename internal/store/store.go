package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"subtitleqc/internal/domain"
)

type Snapshot struct {
	SchemaVersion int                                `json:"schemaVersion"`
	Sequence      int64                              `json:"sequence"`
	Packages      map[string]*domain.SubtitlePackage `json:"packages"`
	Idempotency   map[string]json.RawMessage         `json:"idempotency,omitempty"`
}

type Store struct {
	dir            string
	mu             sync.Mutex
	snapshot       Snapshot
	events         []domain.AuditEvent
	idem           map[string]json.RawMessage
	eventFile      *os.File
	snapshotLoaded bool
}

type EventRecord struct {
	Type    string
	Actor   string
	Payload map[string]any
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, idem: map[string]json.RawMessage{}, snapshot: Snapshot{SchemaVersion: 1, Packages: map[string]*domain.SubtitlePackage{}, Idempotency: map[string]json.RawMessage{}}}
	if err := s.loadSnapshot(); err != nil {
		return nil, err
	}
	if err := s.loadEvents(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	s.eventFile = f
	return s, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eventFile != nil {
		return s.eventFile.Close()
	}
	return nil
}
func (s *Store) loadSnapshot() error {
	b, err := os.ReadFile(filepath.Join(s.dir, "snapshot.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &s.snapshot); err != nil {
		return fmt.Errorf("快照损坏: %w", err)
	}
	if s.snapshot.SchemaVersion != 1 {
		return fmt.Errorf("不支持的快照版本")
	}
	if s.snapshot.Packages == nil {
		s.snapshot.Packages = map[string]*domain.SubtitlePackage{}
	}
	if s.snapshot.Idempotency == nil {
		s.snapshot.Idempotency = map[string]json.RawMessage{}
	}
	s.idem = s.snapshot.Idempotency
	s.snapshotLoaded = true
	return nil
}
func (s *Store) loadEvents() error {
	f, err := os.Open(filepath.Join(s.dir, "events.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	var prev string
	var seq int64
	for sc.Scan() {
		var e domain.AuditEvent
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			return fmt.Errorf("事件日志损坏")
		}
		if e.Sequence != seq+1 {
			return fmt.Errorf("事件序号不连续")
		}
		raw, _ := json.Marshal(struct {
			Sequence  int64          `json:"sequence"`
			ID        string         `json:"id"`
			PackageID string         `json:"packageId"`
			Type      string         `json:"type"`
			Actor     string         `json:"actor"`
			At        interface{}    `json:"at"`
			Payload   map[string]any `json:"payload"`
			Prev      string         `json:"prev"`
		}{e.Sequence, e.ID, e.PackageID, e.Type, e.Actor, e.At, e.Payload, prev})
		sum := sha256.Sum256(raw)
		if hex.EncodeToString(sum[:]) != e.Checksum {
			return fmt.Errorf("事件校验链错误")
		}
		prev = e.Checksum
		seq = e.Sequence
		s.events = append(s.events, e)
	}
	if err = sc.Err(); err != nil {
		return err
	}
	if s.snapshotLoaded && s.snapshot.Sequence != seq {
		return fmt.Errorf("快照序号与日志不一致")
	}
	s.snapshot.Sequence = seq
	return nil
}

func (s *Store) Get(id string) (*domain.SubtitlePackage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.snapshot.Packages[id]
	if !ok {
		return nil, false
	}
	cp := clonePackage(p)
	return cp, true
}
func (s *Store) List() []*domain.SubtitlePackage {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*domain.SubtitlePackage, 0, len(s.snapshot.Packages))
	for _, p := range s.snapshot.Packages {
		out = append(out, clonePackage(p))
	}
	return out
}
func (s *Store) Events(id string) []domain.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []domain.AuditEvent
	for _, e := range s.events {
		if id == "" || e.PackageID == id {
			out = append(out, e)
		}
	}
	return out
}

// idempotencyRecord captures the request that first reserved an idempotency key,
// so later attempts to reuse the same key for a different package or action can
// be rejected instead of being mistaken for a legal replay.
type idempotencyRecord struct {
	PackageID string `json:"packageId"`
	Action    string `json:"action"`
	Result    string `json:"result"`
	Error     string `json:"error,omitempty"`
}

func (s *Store) CheckIdempotency(key, packageID, action string) (replayed bool, err error) {
	if key == "" {
		return false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.idem[key]
	if !ok {
		return false, nil
	}
	var prior idempotencyRecord
	if e := json.Unmarshal(raw, &prior); e != nil {
		return false, fmt.Errorf("幂等记录损坏")
	}
	if prior.Error != "" {
		return false, errors.New(prior.Error)
	}
	if prior.PackageID != packageID || prior.Action != action {
		return false, fmt.Errorf("幂等键已用于其他请求")
	}
	return true, nil
}

func (s *Store) HasIdempotency(key string) bool {
	if key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.idem[key]
	return ok
}

func (s *Store) Commit(id, key string, expected int, p *domain.SubtitlePackage, typ, actor string, payload map[string]any) error {
	return s.CommitMany(id, key, expected, p, []EventRecord{{Type: typ, Actor: actor, Payload: payload}})
}

func (s *Store) CommitMany(id, key string, expected int, p *domain.SubtitlePackage, records []EventRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	action := ""
	if len(records) > 0 {
		action = records[0].Type
	}
	if key != "" {
		if raw, ok := s.idem[key]; ok {
			var prior idempotencyRecord
			if e := json.Unmarshal(raw, &prior); e != nil {
				return fmt.Errorf("幂等记录损坏")
			}
			if prior.Error != "" {
				return errors.New(prior.Error)
			}
			if prior.PackageID != id || prior.Action != action {
				return fmt.Errorf("幂等键已用于其他请求")
			}
			return nil
		}
	}
	current := 0
	if old, ok := s.snapshot.Packages[id]; ok {
		current = old.Version
	}
	if expected >= 0 && current != expected {
		return fmt.Errorf("版本冲突: 期望%d，当前%d", expected, current)
	}
	if len(records) == 0 {
		return fmt.Errorf("提交事件不能为空")
	}
	s.snapshot.Packages[id] = clonePackage(p)
	for _, record := range records {
		e := s.newEvent(id, record.Type, record.Actor, record.Payload)
		if err := s.appendEvent(e); err != nil {
			return err
		}
	}
	if key != "" {
		raw, _ := json.Marshal(idempotencyRecord{PackageID: id, Action: action, Result: "ok"})
		s.idem[key] = raw
		s.snapshot.Idempotency[key] = raw
	}
	if err := s.writeSnapshot(); err != nil {
		return err
	}
	return nil
}

func (s *Store) newEvent(id, typ, actor string, payload map[string]any) domain.AuditEvent {
	seq := s.snapshot.Sequence + 1
	prev := ""
	if n := len(s.events); n > 0 {
		prev = s.events[n-1].Checksum
	}
	e := domain.AuditEvent{Sequence: seq, ID: fmt.Sprintf("evt-%d", seq), PackageID: id, Type: typ, Actor: actor, At: now(), Payload: payload}
	raw, _ := json.Marshal(struct {
		Sequence  int64          `json:"sequence"`
		ID        string         `json:"id"`
		PackageID string         `json:"packageId"`
		Type      string         `json:"type"`
		Actor     string         `json:"actor"`
		At        interface{}    `json:"at"`
		Payload   map[string]any `json:"payload"`
		Prev      string         `json:"prev"`
	}{e.Sequence, e.ID, e.PackageID, e.Type, e.Actor, e.At, e.Payload, prev})
	sum := sha256.Sum256(raw)
	e.Checksum = hex.EncodeToString(sum[:])
	return e
}
func (s *Store) appendEvent(e domain.AuditEvent) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err = s.eventFile.Write(append(b, '\n')); err != nil {
		return err
	}
	if err = s.eventFile.Sync(); err != nil {
		return err
	}
	s.events = append(s.events, e)
	s.snapshot.Sequence = e.Sequence
	return nil
}
func (s *Store) writeSnapshot() error {
	b, err := json.MarshalIndent(s.snapshot, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, "snapshot.tmp")
	if err = os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if f, err := os.OpenFile(tmp, os.O_RDONLY, 0); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
	return os.Rename(tmp, filepath.Join(s.dir, "snapshot.json"))
}
func clonePackage(p *domain.SubtitlePackage) *domain.SubtitlePackage {
	b, _ := json.Marshal(p)
	var x domain.SubtitlePackage
	_ = json.Unmarshal(b, &x)
	return &x
}
func now() time.Time { return time.Now().UTC() }
