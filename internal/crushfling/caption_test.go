package crushfling

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCaptionFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write caption file: %v", err)
	}
}

func TestCaptionPeekDoesNotConsumeUntilRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "caption.txt")
	writeCaptionFile(t, path, "# keep this comment\nfirst caption\nsecond caption\n")

	cm, err := NewCaptionManager(path)
	if err != nil {
		t.Fatalf("NewCaptionManager: %v", err)
	}
	if got := cm.Count(); got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}

	line, err := cm.Peek()
	if err != nil {
		t.Fatalf("Peek: %v", err)
	}
	if line != "first caption" {
		t.Fatalf("Peek() = %q, want %q", line, "first caption")
	}
	if got := cm.Count(); got != 2 {
		t.Fatalf("Peek consumed a line: Count() = %d, want 2", got)
	}

	if err := cm.RemoveLine(line); err != nil {
		t.Fatalf("RemoveLine: %v", err)
	}
	if got := cm.Count(); got != 1 {
		t.Fatalf("Count() after remove = %d, want 1", got)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(raw), "# keep this comment") {
		t.Fatalf("comment line was lost: %q", string(raw))
	}
	if strings.Contains(string(raw), "first caption") {
		t.Fatalf("used caption still in file: %q", string(raw))
	}
	if !strings.Contains(string(raw), "second caption") {
		t.Fatalf("unused caption disappeared: %q", string(raw))
	}
}

func TestCaptionMissingFileSeedsTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "caption.txt")
	cm, err := NewCaptionManager(path)
	if err != nil {
		t.Fatalf("NewCaptionManager: %v", err)
	}
	if cm.Count() == 0 {
		t.Fatal("expected the template to seed usable captions")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("caption.txt was not created: %v", err)
	}
}

func TestCaptionPickAndRemoveDeletesImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "caption.txt")
	writeCaptionFile(t, path, "only line\n")
	cm, err := NewCaptionManager(path)
	if err != nil {
		t.Fatalf("NewCaptionManager: %v", err)
	}
	line, err := cm.PickAndRemove()
	if err != nil {
		t.Fatalf("PickAndRemove: %v", err)
	}
	if line != "only line" {
		t.Fatalf("PickAndRemove() = %q", line)
	}
	if cm.Count() != 0 {
		t.Fatalf("Count() = %d, want 0", cm.Count())
	}
	if _, err := cm.Peek(); err == nil {
		t.Fatal("Peek should fail when no usable caption is left")
	}
}

func TestPictureManagerMovesUsedPicture(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pictures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("not-a-real-jpeg"), 0o644); err != nil {
		t.Fatalf("write picture: %v", err)
	}

	pm, err := NewPictureManager(dir)
	if err != nil {
		t.Fatalf("NewPictureManager: %v", err)
	}
	if pm.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", pm.Count())
	}

	picked, err := pm.PickRandom()
	if err != nil {
		t.Fatalf("PickRandom: %v", err)
	}
	if err := pm.MarkUsed(picked); err != nil {
		t.Fatalf("MarkUsed: %v", err)
	}
	if pm.Count() != 0 {
		t.Fatalf("Count() after MarkUsed = %d, want 0", pm.Count())
	}
	if _, err := os.Stat(filepath.Join(dir, "used", "a.jpg")); err != nil {
		t.Fatalf("picture was not moved into used/: %v", err)
	}
}

func TestEnsureDataLayoutCreatesFoldersAndConfig(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureDataLayout(dir); err != nil {
		t.Fatalf("EnsureDataLayout: %v", err)
	}
	// A fresh folder gets bio.txt (the current name); caption.txt is only
	// created for folders that already used it.
	for _, rel := range []string{BioFileName, "pictures", filepath.Join("pictures", "used"), EndpointConfigFileName} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("%s missing: %v", rel, err)
		}
	}
	if BioPath(dir) != filepath.Join(dir, BioFileName) {
		t.Fatalf("BioPath on a fresh folder = %q", BioPath(dir))
	}
	// Re-running must not destroy operator edits.
	custom := filepath.Join(dir, BioFileName)
	if err := os.WriteFile(custom, []byte("custom only\n"), 0o644); err != nil {
		t.Fatalf("write custom: %v", err)
	}
	if err := EnsureDataLayout(dir); err != nil {
		t.Fatalf("EnsureDataLayout rerun: %v", err)
	}
	raw, _ := os.ReadFile(custom)
	if strings.TrimSpace(string(raw)) != "custom only" {
		t.Fatalf("bio.txt was overwritten: %q", string(raw))
	}
}

func TestCaptionReservationsAreExclusiveAndReleasedOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "caption.txt")
	writeCaptionFile(t, path, "a caption\nb caption\n")

	cm, err := NewCaptionManager(path)
	if err != nil {
		t.Fatalf("NewCaptionManager: %v", err)
	}

	first, err := cm.Next()
	if err != nil || first != "a caption" {
		t.Fatalf("Next() = %q, %v", first, err)
	}
	second, err := cm.Next()
	if err != nil || second != "b caption" {
		t.Fatalf("second Next() = %q, %v", second, err)
	}
	if _, err := cm.Next(); err == nil {
		t.Fatal("a third reservation must fail while both lines are in flight")
	}

	// A failed account releases its line and the line becomes usable again.
	cm.Release(second)
	again, err := cm.Next()
	if err != nil || again != "b caption" {
		t.Fatalf("after Release, Next() = %q, %v", again, err)
	}

	// The verified account removes its line from the file.
	if err := cm.RemoveLine(first); err != nil {
		t.Fatalf("RemoveLine: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "a caption") {
		t.Fatalf("removed caption still on disk: %q", string(raw))
	}
	if !strings.Contains(string(raw), "b caption") {
		t.Fatalf("in-flight caption missing from disk: %q", string(raw))
	}
}

func TestBioPathPrefersBioAndFallsBackToCaption(t *testing.T) {
	dir := t.TempDir()
	// Only caption.txt exists (a data folder from an older build).
	if err := os.WriteFile(filepath.Join(dir, "caption.txt"), []byte("legacy line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := BioPath(dir); got != filepath.Join(dir, "caption.txt") {
		t.Fatalf("BioPath = %q, want caption.txt fallback", got)
	}
	// A fresh folder gets bio.txt, not caption.txt.
	if got := BioPath(t.TempDir()); got != filepath.Join(t.TempDir(), BioFileName) && filepath.Base(got) != BioFileName {
		t.Fatalf("BioPath on a fresh folder = %q, want bio.txt", got)
	}
	// Once bio.txt exists it wins.
	if err := os.WriteFile(filepath.Join(dir, BioFileName), []byte("new line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := BioPath(dir); got != filepath.Join(dir, BioFileName) {
		t.Fatalf("BioPath = %q, want bio.txt", got)
	}
}

func TestImportBioFileNormalizesAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "bios.txt")
	body := "\ufefffirst line\r\n\r\nfirst line\n# a comment\nsecond line \n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := ImportBioFile(dir, src)
	if err != nil {
		t.Fatalf("ImportBioFile: %v", err)
	}
	if info.Total != 2 || info.Remaining != 2 {
		t.Fatalf("imported %d/%d lines, want 2", info.Total, info.Remaining)
	}
	if info.FileName != BioFileName {
		t.Fatalf("file name = %q, want %q", info.FileName, BioFileName)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, BioFileName))
	if string(raw) != "first line\nsecond line\n" {
		t.Fatalf("normalised file = %q", string(raw))
	}
	// Empty input is rejected, not written.
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("\n# only comments\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportBioFile(dir, empty); err == nil {
		t.Fatal("importing a file with no usable line must fail")
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, BioFileName)); string(raw) != "first line\nsecond line\n" {
		t.Fatalf("a failed import must not touch the old list: %q", string(raw))
	}
}

// A used bio line disappears from the active file; a released one survives.
func TestBioLineIsDeletedOnlyAfterCommit(t *testing.T) {
	dir := t.TempDir()
	info, err := ImportBioFile(dir, writeTempBio(t, "line one\nline two\n"))
	if err != nil || info.Remaining != 2 {
		t.Fatalf("import: %+v %v", info, err)
	}
	cm, err := NewCaptionManager(BioPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	line, err := cm.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cm.Next(); err != nil {
		t.Fatalf("second reservation: %v", err)
	}
	cm.Release(line)
	if err := cm.RemoveLine(line); err != nil {
		t.Fatalf("RemoveLine: %v", err)
	}
	after, err := ReadBioInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.Remaining != 1 || after.Preview[0] == line {
		t.Fatalf("after removal: %+v", after)
	}
}

func writeTempBio(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bio-source.txt")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
