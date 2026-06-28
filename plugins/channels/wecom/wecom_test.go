// SPDX-License-Identifier: MIT

package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agezt/agezt/kernel/channel"
)

// a valid 43-char EncodingAESKey (base64 of 32 bytes, sans padding).
func testAESKey() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))[:43]
}

func encryptTestPayload(c *Channel, msg string, randPrefix []byte, receiveID string) (string, error) {
	if len(c.aesKey) != 32 {
		return "", fmt.Errorf("wecom: AES key not configured")
	}
	var buf bytes.Buffer
	buf.Write(randPrefix[:16])
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(msg)))
	buf.WriteString(msg)
	buf.WriteString(receiveID)
	padded := pkcs7PadTest(buf.Bytes(), aes.BlockSize)
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return "", err
	}
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, c.aesKey[:aes.BlockSize]).CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

func pkcs7PadTest(b []byte, blockSize int) []byte {
	pad := blockSize - len(b)%blockSize
	return append(b, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func TestParseMessageMedia(t *testing.T) {
	img, ok := parseMessage([]byte(`<xml><FromUserName>u1</FromUserName><MsgType>image</MsgType><MediaId>MID1</MediaId><MsgId>2</MsgId></xml>`))
	if !ok || img.mediaType != "image" || img.mediaID != "MID1" {
		t.Fatalf("image = %+v ok=%v", img, ok)
	}
	voc, ok := parseMessage([]byte(`<xml><FromUserName>u1</FromUserName><MsgType>voice</MsgType><MediaId>MID2</MediaId><MsgId>3</MsgId></xml>`))
	if !ok || voc.mediaType != "audio" || voc.mediaID != "MID2" {
		t.Fatalf("voice = %+v ok=%v", voc, ok)
	}
	if _, ok := parseMessage([]byte(`<xml><MsgType>location</MsgType></xml>`)); ok {
		t.Fatal("location should be dropped")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := New(Config{AESKey: testAESKey()})
	inner := `<xml><FromUserName><![CDATA[user1]]></FromUserName><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[hello]]></Content><MsgId>99</MsgId></xml>`
	enc, err := encryptTestPayload(c, inner, make([]byte, 16), "corpid")
	if err != nil {
		t.Fatal(err)
	}
	msg, recv, err := c.decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if string(msg) != inner || recv != "corpid" {
		t.Fatalf("decrypt = %q recv=%q", msg, recv)
	}
	m, ok := parseMessage(msg)
	if !ok || m.sender != "user1" || m.text != "hello" || m.id != "99" {
		t.Fatalf("parseMessage = %+v ok=%v", m, ok)
	}
}

func TestSignatureStable(t *testing.T) {
	a := signature("tok", "100", "nonce", "ENC")
	b := signature("tok", "100", "nonce", "ENC")
	if a == "" || a != b {
		t.Fatalf("signature unstable: %q %q", a, b)
	}
	if signature("tok", "100", "nonce", "ENC") == signature("tok", "101", "nonce", "ENC") {
		t.Fatal("signature should depend on timestamp")
	}
}

func TestSendUsesAccessToken(t *testing.T) {
	var tokenHits, sendHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			tokenHits++
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "AT-1", "expires_in": 7200})
			return
		}
		if r.URL.Path == "/cgi-bin/message/send" {
			sendHits++
			if r.URL.Query().Get("access_token") != "AT-1" {
				t.Errorf("missing access_token: %s", r.URL.RawQuery)
			}
			b, _ := io.ReadAll(r.Body)
			var p map[string]any
			_ = json.Unmarshal(b, &p)
			if p["touser"] != "user1" {
				t.Errorf("touser = %v", p["touser"])
			}
		}
	}))
	defer srv.Close()
	c := New(Config{CorpID: "c", CorpSecret: "s", AgentID: "1", AESKey: testAESKey(), APIBase: srv.URL, HTTPClient: srv.Client()})
	if err := c.Send(context.Background(), channel.Outbound{ChannelID: "user1", Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	if tokenHits != 1 || sendHits != 1 {
		t.Fatalf("tokenHits=%d sendHits=%d", tokenHits, sendHits)
	}
}
