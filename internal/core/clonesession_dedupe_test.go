package core

// 会话克隆去重 + 切换快照保留策略 的单测（2026-11-05）。
//
// 背景：克隆无去重时，A→B→A 来回切换会把克隆再克隆，会话列表按切换次数
// 滚雪球（用户实测切 5 次同一对话 5 份）；快照 sessions-*.db 每次切换一份、
// 只增不删。修复：正文指纹去重 + 快照保留最近 5 份。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCloneDedupeRoundTrip 来回切换不滚雪球：
// A→B 克隆 2 条；B→A 时 B 名下含 A 会话的克隆，指纹与 A 原版相同应跳过，
// A 只新增 s-b1 的副本；再 A→B 第二轮应全部跳过（0 条）。
func TestCloneDedupeRoundTrip(t *testing.T) {
	root := newSessionTestRoot(t)
	dbPath := filepath.Join(root, "workbuddy.db")

	// 第一跳 A→B：uid-a 的 s-a1/s-a2 克隆给 uid-b
	n1, err := cloneClientSessionsAt(root, t.TempDir(), "uid-a", "uid-b")
	if err != nil {
		t.Fatalf("A→B 克隆失败: %v", err)
	}
	if n1 != 2 {
		t.Fatalf("A→B 应克隆 2 条，实际 %d", n1)
	}

	// 第二跳 B→A：uid-b 名下 3 条本地（s-b1 + 两个克隆），其中克隆与
	// uid-a 原版指纹相同应跳过，只有 s-b1 是新内容
	n2, err := cloneClientSessionsAt(root, t.TempDir(), "uid-b", "uid-a")
	if err != nil {
		t.Fatalf("B→A 克隆失败: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("B→A 应只克隆 1 条（s-b1），实际 %d——去重失效，会话在滚雪球", n2)
	}

	// 第三跳 A→B：双方内容已对称，应全部跳过
	n3, err := cloneClientSessionsAt(root, t.TempDir(), "uid-a", "uid-b")
	if err != nil {
		t.Fatalf("A→B 二轮克隆失败: %v", err)
	}
	if n3 != 0 {
		t.Fatalf("A→B 二轮应克隆 0 条，实际 %d", n3)
	}

	// 终态：uid-a 本地 = s-a1/s-a2 原版 + s-claw（不克隆但计入 local）+ 1 个 s-b1 副本 = 4；
	// uid-b = s-b1 原版 + 2 克隆 = 3
	if got := countSessions(t, dbPath, "uid-a", "local"); got != 4 {
		t.Fatalf("uid-a 本地会话应为 4，实际 %d", got)
	}
	if got := countSessions(t, dbPath, "uid-b", "local"); got != 3 {
		t.Fatalf("uid-b 本地会话应为 3，实际 %d", got)
	}
}

// TestCloneDedupeIdenticalContent 不同 id 但正文除自身 id 外完全相同的两条源
//（模拟历史遗留的重复副本），同批克隆时只应落一份。
func TestCloneDedupeIdenticalContent(t *testing.T) {
	root := newSessionTestRoot(t)
	// s-a1 的历史遗留副本 s-a1-dup：正文与 s-a1 仅 id 不同
	ws := filepath.Join(root, "projects", "Users-me-work")
	body, _ := os.ReadFile(filepath.Join(ws, "s-a1.jsonl"))
	dup := strings.Replace(string(body), "s-a1", "s-a1-dup", -1)
	if err := os.WriteFile(filepath.Join(ws, "s-a1-dup.jsonl"), []byte(dup), 0o600); err != nil {
		t.Fatalf("写遗留副本失败: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "workbuddy.db"))
	if err != nil {
		t.Fatalf("打开库失败: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO sessions(id, cwd, user_id, transport, created_at, updated_at)
		VALUES('s-a1-dup','/Users/me/work','uid-a','local',1000,2000)`); err != nil {
		t.Fatalf("插遗留副本行失败: %v", err)
	}

	n, err := cloneClientSessionsAt(root, t.TempDir(), "uid-a", "uid-b")
	if err != nil {
		t.Fatalf("克隆失败: %v", err)
	}
	// s-a1 与 s-a1-dup 同内容：只克隆一份；s-a2 一份 → 共 2
	if n != 2 {
		t.Fatalf("同内容源应只克隆一份（共 2 条），实际 %d", n)
	}
}

// TestPruneSwitchSnapshots 快照保留最近 5 份：预置 8 份旧快照 + 克隆新增 1 份，
// 克隆成功后应只剩 5 份且是最新的。
func TestPruneSwitchSnapshots(t *testing.T) {
	root := newSessionTestRoot(t)
	backupDir := t.TempDir()
	for i := 1; i <= 8; i++ {
		name := filepath.Join(backupDir, fmt.Sprintf("sessions-2026110%d-000000.db", i))
		if err := os.WriteFile(name, []byte("fake"), 0o600); err != nil {
			t.Fatalf("预置快照失败: %v", err)
		}
	}
	if _, err := cloneClientSessionsAt(root, backupDir, "uid-a", "uid-b"); err != nil {
		t.Fatalf("克隆失败: %v", err)
	}
	snaps, _ := filepath.Glob(filepath.Join(backupDir, "sessions-*.db"))
	if len(snaps) != 5 {
		t.Fatalf("应保留 5 份快照，实际 %d", len(snaps))
	}
	// 留下的必须是最新的（含本次克隆生成的，删掉的是最老的 4 份）
	for _, p := range snaps {
		base := filepath.Base(p)
		if strings.HasPrefix(base, "sessions-20261101-") || strings.HasPrefix(base, "sessions-20261102-") ||
			strings.HasPrefix(base, "sessions-20261103-") || strings.HasPrefix(base, "sessions-20261104-") {
			t.Fatalf("最老的快照应被清理却保留了下来: %s", base)
		}
	}
}
