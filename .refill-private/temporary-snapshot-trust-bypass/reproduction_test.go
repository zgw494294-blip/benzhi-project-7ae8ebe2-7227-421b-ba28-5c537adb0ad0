package temporarysnapshot_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"subtitleqc/internal/domain"
	"subtitleqc/internal/store"
)

func TestTemporarySnapshotRecoveryPreservesValidatedState(t *testing.T) {
	t.Run("有效临时快照接管完整状态", func(t *testing.T) {
		dir, _, latest := buildTwoVersionStore(t)
		writeRecoveryCandidates(t, dir, latest)

		recovered, err := store.Open(dir)
		if err != nil {
			t.Fatalf("有效临时快照恢复失败: %v", err)
		}
		defer recovered.Close()
		p, ok := recovered.Get("pkg-recovery")
		if !ok || p.Version != 2 || p.Status != domain.StatusPrepared {
			t.Errorf("临时快照未恢复最新投影: %+v", p)
		}
		if !recovered.HasIdempotency("prepare-recovery") {
			t.Errorf("临时快照恢复丢失幂等记录")
		}
	})

	t.Run("过期临时快照不能静默回退", func(t *testing.T) {
		dir, stale, _ := buildTwoVersionStore(t)
		writeRecoveryCandidates(t, dir, stale)

		recovered, err := store.Open(dir)
		if err != nil {
			return
		}
		defer recovered.Close()
		p, ok := recovered.Get("pkg-recovery")
		if !ok || p.Version != 2 || p.Status != domain.StatusPrepared {
			t.Errorf("过期临时快照使投影从 prepared 回退: %+v", p)
		}
	})

	t.Run("临时快照必须校验schemaVersion", func(t *testing.T) {
		dir, _, latest := buildTwoVersionStore(t)
		var candidate map[string]any
		if err := json.Unmarshal(latest, &candidate); err != nil {
			t.Fatal(err)
		}
		candidate["schemaVersion"] = 99
		invalid, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		writeRecoveryCandidates(t, dir, invalid)
		if recovered, err := store.Open(dir); err == nil {
			recovered.Close()
			t.Errorf("错误 schemaVersion 的临时快照被接受")
		}
	})
}

func buildTwoVersionStore(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p := &domain.SubtitlePackage{
		ID: "pkg-recovery", ProgramTitle: "恢复测试", EpisodeCode: "REC-01",
		AudioDurationMs: 5000, Language: "zh-CN", DeliveryStandard: "WebVTT",
		Status: domain.StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := st.Commit(p.ID, "create-recovery", 0, p, domain.EventPackageCreated, "tester", map[string]any{"status": domain.StatusDraft}); err != nil {
		t.Fatal(err)
	}
	stale, err := os.ReadFile(st.SnapshotPath())
	if err != nil {
		t.Fatal(err)
	}
	p.Status = domain.StatusPrepared
	p.Version = 2
	p.Cues = []domain.CaptionCue{{ID: "cue-1", PackageID: p.ID, Sequence: 1, StartMs: 0, EndMs: 1000, Speaker: "主持人", Text: "恢复内容", Revision: 1}}
	if err := st.Commit(p.ID, "prepare-recovery", 1, p, domain.EventCuesPrepared, "tester", map[string]any{"cueCount": 1}); err != nil {
		t.Fatal(err)
	}
	latest, err := os.ReadFile(st.SnapshotPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, stale, latest
}

func writeRecoveryCandidates(t *testing.T, dir string, temporary []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "snapshot.tmp"), temporary, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
}
