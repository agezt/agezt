// SPDX-License-Identifier: MIT

package nostr

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	btcec "github.com/btcsuite/btcd/btcec/v2"
)

// NIP-04 encrypted direct messages: AES-256-CBC keyed by the ECDH shared secret
// (the raw secp256k1 x-coordinate, which btcec.GenerateSharedSecret returns per
// RFC5903 §9). The wire content is "<base64 ciphertext>?iv=<base64 iv>".
//
// NIP-04 is the most widely-supported DM scheme; NIP-44/NIP-17 are a possible
// future upgrade.

// nip04Encrypt encrypts plaintext for pub using the shared secret with our priv.
func nip04Encrypt(priv *btcec.PrivateKey, pub *btcec.PublicKey, plaintext string) (string, error) {
	key := btcec.GenerateSharedSecret(priv, pub) // 32-byte x-coordinate → AES-256 key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	return base64.StdEncoding.EncodeToString(ct) + "?iv=" + base64.StdEncoding.EncodeToString(iv), nil
}

// nip04Decrypt decrypts a "<ct>?iv=<iv>" NIP-04 content from pub.
func nip04Decrypt(priv *btcec.PrivateKey, pub *btcec.PublicKey, content string) (string, error) {
	ctB64, ivB64, ok := strings.Cut(content, "?iv=")
	if !ok {
		return "", fmt.Errorf("nip04: missing iv")
	}
	ct, err := base64.StdEncoding.DecodeString(ctB64)
	if err != nil {
		return "", err
	}
	iv, err := base64.StdEncoding.DecodeString(ivB64)
	if err != nil {
		return "", err
	}
	key := btcec.GenerateSharedSecret(priv, pub)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	if len(iv) != aes.BlockSize || len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return "", fmt.Errorf("nip04: bad ciphertext")
	}
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	out, err := pkcs7Unpad(pt, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func pkcs7Pad(b []byte, size int) []byte {
	n := size - len(b)%size
	pad := make([]byte, n)
	for i := range pad {
		pad[i] = byte(n)
	}
	return append(b, pad...)
}

func pkcs7Unpad(b []byte, size int) ([]byte, error) {
	if len(b) == 0 || len(b)%size != 0 {
		return nil, fmt.Errorf("nip04: bad padding length")
	}
	n := int(b[len(b)-1])
	if n == 0 || n > size || n > len(b) {
		return nil, fmt.Errorf("nip04: bad padding")
	}
	for _, c := range b[len(b)-n:] {
		if int(c) != n {
			return nil, fmt.Errorf("nip04: bad padding bytes")
		}
	}
	return b[:len(b)-n], nil
}
