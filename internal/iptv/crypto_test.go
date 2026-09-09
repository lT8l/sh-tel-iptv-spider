package iptv

import (
	"bytes"
	"crypto/aes"
	"encoding/hex"
	"testing"
)

func TestAESECBEncryptMatchesLegacyVector(t *testing.T) {
	got, err := aesECBEncrypt([]byte("data12345"), []byte("123456"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "58ead24a41db42bf7f9b6d53195cdf69"
	if encoded := hex.EncodeToString(got); encoded != want {
		t.Fatalf("ciphertext=%s, want %s", encoded, want)
	}
}

func TestPKCS7PaddingUsesAESBlockSize(t *testing.T) {
	got := pkcs7Padding([]byte("x"), aes.BlockSize)
	if len(got) != aes.BlockSize {
		t.Fatalf("padded length=%d, want %d", len(got), aes.BlockSize)
	}
	wantPadding := bytes.Repeat([]byte{aes.BlockSize - 1}, aes.BlockSize-1)
	if !bytes.Equal(got[1:], wantPadding) {
		t.Fatalf("padding=%x, want %x", got[1:], wantPadding)
	}
}
