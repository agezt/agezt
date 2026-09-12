// SPDX-License-Identifier: MIT

// WeCom channel: crypto helpers (signature + decrypt + pkcs7Unpad).
// Code extracted from wecom.go during the Day-112 god-file split.
// Public API unchanged.
package wecom


import (
	"fmt"
	"sort"
	"strings"

	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
)

func signature(token, timestamp, nonce, encrypt string) string {
	arr := []string{token, timestamp, nonce, encrypt}
	sort.Strings(arr)
	h := sha1.New()
	h.Write([]byte(strings.Join(arr, "")))
	return hex.EncodeToString(h.Sum(nil))
}

// decrypt reverses the WXBizMsgCrypt AES-256-CBC scheme:
// plain = rand(16) || msgLen(4, big-endian) || msg || receiveid.
func (c *Channel) decrypt(encrypt string) (msg []byte, receiveID string, err error) {
	if len(c.aesKey) != 32 {
		return nil, "", fmt.Errorf("wecom: AES key not configured")
	}
	cipherText, err := base64.StdEncoding.DecodeString(encrypt)
	if err != nil {
		return nil, "", err
	}
	if len(cipherText) < aes.BlockSize || len(cipherText)%aes.BlockSize != 0 {
		return nil, "", fmt.Errorf("wecom: bad ciphertext length")
	}
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, "", err
	}
	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, c.aesKey[:aes.BlockSize]).CryptBlocks(plain, cipherText)
	plain, err = pkcs7Unpad(plain)
	if err != nil {
		return nil, "", err
	}
	if len(plain) < 20 {
		return nil, "", fmt.Errorf("wecom: short plaintext")
	}
	msgLen := binary.BigEndian.Uint32(plain[16:20])
	if int(20+msgLen) > len(plain) {
		return nil, "", fmt.Errorf("wecom: bad msg length")
	}
	return plain[20 : 20+msgLen], string(plain[20+msgLen:]), nil
}

func pkcs7Unpad(b []byte) ([]byte, error) {
	n := len(b)
	if n == 0 {
		return nil, fmt.Errorf("wecom: empty plaintext")
	}
	pad := int(b[n-1])
	if pad < 1 || pad > 32 || pad > n {
		return nil, fmt.Errorf("wecom: bad padding")
	}
	return b[:n-pad], nil
}

// parseMessage reads the decrypted inner XML: <xml><FromUserName/><Content/>
// <MsgType/><MsgId/><MediaId/></xml>. Text plus image/voice media messages are kept.
func parseMessage(plain []byte) (inbound, bool) {
	var x struct {
		FromUserName string `xml:"FromUserName"`
		MsgType      string `xml:"MsgType"`
		Content      string `xml:"Content"`
		MsgID        string `xml:"MsgId"`
		MediaID      string `xml:"MediaId"`
	}
	if err := xml.Unmarshal(plain, &x); err != nil {
		return inbound{}, false
	}
	in := inbound{sender: x.FromUserName, text: strings.TrimSpace(x.Content), id: x.MsgID}
	switch x.MsgType {
	case "", "text":
	case "image":
		in.mediaID, in.mediaType = x.MediaID, "image"
	case "voice":
		in.mediaID, in.mediaType = x.MediaID, "audio"
	default:
		return inbound{}, false
	}
	return in, true
}
