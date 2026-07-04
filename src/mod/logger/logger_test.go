package logger

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestFolder(t *testing.T) string {
	t.Helper()
	folder := t.TempDir()
	if err := Init(folder); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return folder
}

func TestWriteAndRead(t *testing.T) {
	setupTestFolder(t)

	Write("system", "hello world")
	Write("cmd", "line one\nline two\nline three")

	content, truncated, err := ReadLogFile(currentLogName(), 0)
	if err != nil {
		t.Fatalf("ReadLogFile failed: %v", err)
	}
	if truncated {
		t.Error("small file should not be truncated")
	}
	if !strings.Contains(content, "[system] hello world") {
		t.Errorf("missing system entry: %s", content)
	}
	if !strings.Contains(content, "[cmd] line one") {
		t.Errorf("missing cmd entry head: %s", content)
	}
	if !strings.Contains(content, "    line two") {
		t.Errorf("continuation lines should be indented: %s", content)
	}
}

func TestLogCommand(t *testing.T) {
	setupTestFolder(t)

	cmd := exec.Command("echo", "formatting done")
	LogCommand(cmd.Args, "formatting done", nil)
	LogCommand([]string{"sudo", "mkfs.ext4", "/dev/null"}, "mkfs failed output", os.ErrPermission)

	content, _, err := ReadLogFile(currentLogName(), 0)
	if err != nil {
		t.Fatalf("ReadLogFile failed: %v", err)
	}
	if !strings.Contains(content, "$ echo formatting done (ok)") {
		t.Errorf("missing ok command entry: %s", content)
	}
	if !strings.Contains(content, "$ sudo mkfs.ext4 /dev/null (failed: permission denied)") {
		t.Errorf("missing failed command entry: %s", content)
	}
}

func TestReadLogFileTailCap(t *testing.T) {
	setupTestFolder(t)

	for i := 0; i < 100; i++ {
		Write("system", strings.Repeat("x", 100))
	}
	content, truncated, err := ReadLogFile(currentLogName(), 1024)
	if err != nil {
		t.Fatalf("ReadLogFile failed: %v", err)
	}
	if !truncated {
		t.Error("expected the read to be truncated")
	}
	if int64(len(content)) > 1024 {
		t.Errorf("content exceeds cap: %d bytes", len(content))
	}
	// The cut must land on a line boundary
	if !strings.HasPrefix(content, "[") {
		t.Errorf("content should start at a full line, got: %q", content[:20])
	}
}

func TestValidateNameRejectsTraversal(t *testing.T) {
	setupTestFolder(t)

	for _, bad := range []string{"../secret.log", "..%2Fsecret.log", "/etc/passwd", "a/b.log", "no-extension"} {
		if _, _, err := ReadLogFile(bad, 0); err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
	}
}

func TestDeleteProtectsCurrentFile(t *testing.T) {
	setupTestFolder(t)
	Write("system", "keep me")

	if err := DeleteLogFile(currentLogName()); err == nil {
		t.Error("deleting the active log file should fail")
	}
}

func TestCleanupAgeAndSize(t *testing.T) {
	folder := setupTestFolder(t)
	Write("system", "current entry")

	// One file 10 months old, one 1 month old (~1KB each)
	oldFile := filepath.Join(folder, "bokodm-old.log")
	recentFile := filepath.Join(folder, "bokodm-recent.log")
	os.WriteFile(oldFile, []byte(strings.Repeat("o", 1024)), 0644)
	os.WriteFile(recentFile, []byte(strings.Repeat("r", 1024)), 0644)
	os.Chtimes(oldFile, time.Now().AddDate(0, -10, 0), time.Now().AddDate(0, -10, 0))
	os.Chtimes(recentFile, time.Now().AddDate(0, -1, 0), time.Now().AddDate(0, -1, 0))

	// Age-based: 6 month retention drops only the old file
	SetConfig(6, 0)
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("expired file should have been removed")
	}
	if _, err := os.Stat(recentFile); err != nil {
		t.Error("recent file should have survived age cleanup")
	}

	// Size-based: a 2 MB archived file against a 1 MB folder limit gets
	// purged while the current month's file survives
	bigFile := filepath.Join(folder, "bokodm-big.log")
	os.WriteFile(bigFile, make([]byte, 2*1024*1024), 0644)
	os.Chtimes(bigFile, time.Now().AddDate(0, -2, 0), time.Now().AddDate(0, -2, 0))
	SetConfig(0, 1)
	if _, err := os.Stat(bigFile); !os.IsNotExist(err) {
		t.Error("oversized archived file should have been purged")
	}
	if _, err := os.Stat(filepath.Join(folder, currentLogName())); err != nil {
		t.Error("the current log file must never be deleted by cleanup")
	}
}
