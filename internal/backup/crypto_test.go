package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "plain.db")
	dst := filepath.Join(dir, "plain.db.enc")
	out := filepath.Join(dir, "decrypted.db")

	// A real SQLite-ish payload: arbitrary bytes survive, that is what matters.
	payload := []byte("SQLite format 3\x00" + strings.Repeat("loom-backup-payload", 500))
	if err := os.WriteFile(src, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := encryptFile(src, dst, "正确口令"); err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !looksEncrypted(dst) {
		t.Fatal("encrypted file does not carry the magic header")
	}
	if plain, err := os.ReadFile(dst); err == nil && string(plain[:19]) == "SQLite format 3\x00" {
		t.Fatal("ciphertext starts with the SQLite magic: payload is not encrypted")
	}

	if err := decryptFile(dst, out, "正确口令"); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatal("round trip changed the payload")
	}
}

func TestDecryptWrongPassphrase(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.db")
	dst := filepath.Join(dir, "a.db.enc")
	out := filepath.Join(dir, "b.db")
	if err := os.WriteFile(src, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := encryptFile(src, dst, "right"); err != nil {
		t.Fatal(err)
	}
	if err := decryptFile(dst, out, "wrong"); err == nil {
		t.Fatal("wrong passphrase must fail")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("failed decrypt must not leave an output file")
	}
}

func TestDecryptRejectsNonEncryptedFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "raw.db")
	out := filepath.Join(dir, "out.db")
	if err := os.WriteFile(src, []byte("SQLite format 3\x00plain database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if looksEncrypted(src) {
		t.Fatal("plain database misdetected as encrypted")
	}
	if err := decryptFile(src, out, "any"); err == nil {
		t.Fatal("decrypting a non-encrypted file must fail")
	}
}

func TestEncryptRequiresPassphrase(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.db")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := encryptFile(src, filepath.Join(dir, "a.enc"), ""); err == nil {
		t.Fatal("empty passphrase must be rejected before any crypto work")
	}
}

// TestEncryptedSnapshotLifecycle drives the whole path a user would take:
// snapshot with a passphrase, list it, validate it, stage a restore — and
// checks the plaintext never touches the backup directory.
func TestEncryptedSnapshotLifecycle(t *testing.T) {
	database, dbPath := openTestDB(t)
	bdir := filepath.Join(t.TempDir(), "backups")
	svc := NewService(database, dbPath, bdir, 0, 3, "演练口令")

	info, err := svc.Create()
	if err != nil {
		t.Fatalf("create encrypted backup: %v", err)
	}
	if !info.Encrypted || !strings.HasSuffix(info.Name, ".enc") {
		t.Fatalf("snapshot not marked encrypted: %+v", info)
	}
	// The backup directory must not hold a readable database: every file in it
	// either carries the magic header or is not a SQLite file at all.
	entries, _ := os.ReadDir(bdir)
	for _, e := range entries {
		p := filepath.Join(bdir, e.Name())
		if looksEncrypted(p) {
			continue
		}
		head := make([]byte, 16)
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := f.Read(head)
		f.Close()
		if n >= 16 && string(head[:15]) == "SQLite format 3" {
			t.Fatalf("plaintext database in backup dir: %s", e.Name())
		}
	}

	rep, err := svc.ValidateFile(svc.ResolvePath(info.Name))
	if err != nil {
		t.Fatalf("validate encrypted backup: %v", err)
	}
	if !rep.OK {
		t.Fatalf("encrypted backup failed validation: %v", rep.Problems)
	}

	// Wrong passphrase must fail the validate, not silently pass.
	wrong := NewService(database, dbPath, bdir, 0, 3, "错误口令")
	if _, err := wrong.ValidateFile(wrong.ResolvePath(info.Name)); err == nil {
		t.Fatal("validate with wrong passphrase must fail")
	}

	if _, err := svc.StageRestore(info.Name); err != nil {
		t.Fatalf("stage restore: %v", err)
	}
	staged := dbPath + ".restore-pending"
	if looksEncrypted(staged) {
		t.Fatal("staged restore file is encrypted; it must be directly openable")
	}
	stagedRep, err := Validate(staged)
	if err != nil || !stagedRep.OK {
		t.Fatalf("staged file is not a valid database: %v", stagedRep)
	}
}
