package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
)

// Encrypted snapshot format: magic + version + salt + nonce + AES-256-GCM
// ciphertext. The key is derived per-file from a random salt, so reusing a
// passphrase across snapshots does not reuse keys.
const encMagic = "LOOMENC1"
const encSaltLen = 16
const encNonceLen = 12
const encIterations = 210000

var errNotEncrypted = errors.New("file is not a loom encrypted snapshot")

// looksEncrypted sniffs the magic header.
func looksEncrypted(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, len(encMagic))
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	return string(head) == encMagic
}

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, passphrase, salt, encIterations, 32)
}

// encryptFile encrypts src into dst. The plaintext is removed by the caller.
func encryptFile(src, dst, passphrase string) error {
	if passphrase == "" {
		return errors.New("encrypt: empty passphrase")
	}
	plain, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read plaintext snapshot: %w", err)
	}

	salt := make([]byte, encSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("generate salt: %w", err)
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("init gcm: %w", err)
	}
	nonce := make([]byte, encNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	out := make([]byte, 0, len(encMagic)+1+encSaltLen+encNonceLen+len(plain)+gcm.Overhead())
	out = append(out, encMagic...)
	out = append(out, 1) // format version
	out = append(out, salt...)
	out = append(out, nonce...)
	out = gcm.Seal(out, nonce, plain, nil)
	return os.WriteFile(dst, out, 0o600)
}

// decryptFile decrypts src (an encrypted snapshot) into dst.
func decryptFile(src, dst, passphrase string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read encrypted snapshot: %w", err)
	}
	headerLen := len(encMagic) + 1 + encSaltLen + encNonceLen
	if len(data) < headerLen || string(data[:len(encMagic)]) != encMagic {
		return errNotEncrypted
	}
	if data[len(encMagic)] != 1 {
		return fmt.Errorf("unsupported snapshot format version %d", data[len(encMagic)])
	}
	off := len(encMagic) + 1
	salt := data[off : off+encSaltLen]
	off += encSaltLen
	nonce := data[off : off+encNonceLen]
	body := data[headerLen:]

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("init gcm: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return errors.New("解密失败：口令错误或文件已损坏")
	}
	if err := os.WriteFile(dst, plain, 0o600); err != nil {
		return fmt.Errorf("write decrypted snapshot: %w", err)
	}
	return nil
}
