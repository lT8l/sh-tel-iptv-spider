package iptv

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
)

const rsaPrivateKey = `-----BEGIN PRIVATE KEY-----
MIICdwIBADANBgkqhkiG9w0BAQEFAASCAmEwggJdAgEAAoGBAKwXs+z2tv9LDonG
jyDFWLXB9IzubHJglq/WRWMnFSb5koXu6uLpuJ6ax0MN2HG7JmDO5mT8pZn+6ayL
5HklZ43upnOCbsg3K/tFuEv4CCscztJVt/rC2y9NPJhWYfy/GxdywcAbds6mqjTY
QWbvtJv00qac1QOPprgYez4KxU6dAgMBAAECgYAmTnBigthhI1ftGyGo7cS9UJsa
88d3/kAMi+mOFJkEv/D5lyD5uYS66UEJj/9p8XqteeCXAhXqnW9uVQVaYhUWiS9l
Ois73zUpQNBT48kAMAhn7ux6gpgtSaKwSHsbKUs8YoMe0FoPKIPwln7XLhvLCKpm
5knjWD3gtVQEW8mxAQJBAOAyMMmW17fMLeSdpqskvElh63heiyb+qsmkldjYxRhK
gMK4hlI/6CFX5eIWrZUXJ1cf6UOaJFgBXQ/qxRmkSI0CQQDEgVg+jmjopestab4l
tEM3gzls4+eocwRgfzKEouo4SUtEk6BZhPcuk6Pss10Z+9jqiUMgo6TZJHYkfTWg
7kJRAkAPIlQ4x334YkgWzq2Zj/lF2t5SWc966mYNBpc29CsZ4K2gd2RZ2QaKeayC
/pTpI478SqMsdRNO/YiSsn5rpLNhAkEAttCK82/0A/VQjWhiIZvKKRwpUbfZ7qpK
uSe9LQ6QDxuJLdyWApKkkC2FBRJ9nE3kqZZX4Ea+d9HnI91lBjqDcQJBAN9v1q9e
TIM7EPiRNDlEbvJQ39+6PT8zX8EE9CHAz4mBGp4W7szMas2561xi5iN/ZC9ssUiM
jBrqDb4pgDZAFOc=
-----END PRIVATE KEY-----`

type authenticator struct {
	Random       string `json:"Randon"`
	EncryptToken string `json:"EncryToken"`
	UserID       string `json:"UserID"`
	SN           string `json:"SN"`
	IP           string `json:"IP"`
	MAC          string `json:"MAC"`
	MagicCode    string `json:"MagicCode"`
	UpdateTime   string `json:"UpdateTime"`
}

func newAuthenticator(token, uid, sn, ip, mac string) authenticator {
	return authenticator{
		EncryptToken: token,
		UserID:       uid,
		SN:           sn,
		IP:           formatIP(ip),
		MAC:          mac,
		MagicCode:    "CTC",
		UpdateTime:   "20230301175307",
	}
}

func (a authenticator) encryptedString() (string, error) {
	random, err := randomDigits(8)
	if err != nil {
		return "", fmt.Errorf("generate authenticator random: %w", err)
	}
	a.Random = random
	data, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	enc, err := aesECBEncrypt(data, []byte("123456"))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(enc), nil
}

func aesECBEncrypt(data, password []byte) ([]byte, error) {
	key := md5.Sum(password)
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	plain := pkcs7Padding(data, block.BlockSize())
	out := make([]byte, len(plain))
	for offset := 0; offset < len(plain); offset += block.BlockSize() {
		block.Encrypt(out[offset:offset+block.BlockSize()], plain[offset:offset+block.BlockSize()])
	}
	return out, nil
}

func pkcs7Padding(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func privateEncryptToken(token string) (string, error) {
	plain := insertTokenMarker(token)
	if plain == "" {
		return "", fmt.Errorf("invalid UserToken")
	}
	block, _ := pem.Decode([]byte(rsaPrivateKey))
	if block == nil {
		return "", fmt.Errorf("decode RSA private key")
	}
	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse RSA private key: %w", err)
	}
	key, ok := keyAny.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("unexpected RSA private key type")
	}
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.Hash(0), []byte(plain))
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(sig)), nil
}

func insertTokenMarker(token string) string {
	const index = 7
	if len(token) <= index {
		return ""
	}
	return token[:index] + "37AE" + token[index:]
}

func formatIP(ip string) string {
	parts := strings.Split(ip, ".")
	for i, part := range parts {
		for len(part) < 3 {
			part = "0" + part
		}
		parts[i] = part
	}
	return strings.Join(parts, ",")
}

func randomDigits(n int) (string, error) {
	var b strings.Builder
	b.Grow(n)
	limit := big.NewInt(10)
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + v.Int64()))
	}
	return b.String(), nil
}
