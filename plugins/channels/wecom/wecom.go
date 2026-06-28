// SPDX-License-Identifier: MIT

// Package wecom is a two-way WeCom (WeChat Work / 企业微信) channel over an app's
// encrypted callback. WeCom verifies the callback URL with a GET (msg_signature +
// echostr), then POSTs AES-256-CBC-encrypted XML messages. We verify the SHA-1
// message signature, decrypt (WXBizMsgCrypt scheme), and reply via the app
// message-send API using an access_token fetched from the corp id/secret (cached
// until expiry). An empty allowlist is fail-closed; without an Addr the channel
// is send-only.
package wecom

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/channel"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/ulid"
)

const (
	// DefaultPath is the inbound callback route WeCom should POST to.
	DefaultPath    = "/wecom"
	defaultAPIBase = "https://qyapi.weixin.qq.com"
	maxBody        = 1 << 20
	wecomMaxChars  = 2000
	dedupCapacity  = 2048
)

// Config configures the WeCom channel.
type Config struct {
	CorpID     string // corp id
	CorpSecret string // app secret (fetches access_token)
	AgentID    string // app agent id
	Token      string // callback token (signature)
	AESKey     string // callback EncodingAESKey (43 chars)
	Allowlist  channel.Allowlist
	Bus        *bus.Bus
	Handler    channel.InboundHandler
	Addr       string // optional host:port to serve the inbound callback; blank = outbound-only
	Path       string // inbound route (default /wecom)
	APIBase    string // WeCom API base (default https://qyapi.weixin.qq.com); overridable for tests
	HTTPClient *http.Client
}

// Channel is the WeCom surface.
type Channel struct {
	cfg     Config
	path    string
	apiBase string
	aesKey  []byte
	client  *http.Client

	tmu      sync.Mutex
	token    string
	tokenExp time.Time

	dmu  sync.Mutex
	seen map[string]struct{}
	ring []string
}

// New constructs a WeCom channel, applying defaults. AESKey decode errors leave
// aesKey nil; the channel then rejects inbound (outbound still works).
func New(cfg Config) *Channel {
	if cfg.Path == "" {
		cfg.Path = DefaultPath
	}
	base := strings.TrimRight(cfg.APIBase, "/")
	if base == "" {
		base = defaultAPIBase
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	var key []byte
	if cfg.AESKey != "" {
		if k, err := base64.StdEncoding.DecodeString(cfg.AESKey + "="); err == nil {
			key = k
		}
	}
	return &Channel{
		cfg:     cfg,
		path:    cfg.Path,
		apiBase: base,
		aesKey:  key,
		client:  client,
		seen:    make(map[string]struct{}, dedupCapacity),
	}
}

// Name implements channel.Channel.
func (c *Channel) Name() string { return "wecom" }

// Start serves the inbound callback when an Addr is set; otherwise blocks.
func (c *Channel) Start(ctx context.Context) error {
	if c.cfg.Addr == "" {
		<-ctx.Done()
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	srv := &http.Server{
		Addr:              c.cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler exposes the inbound callback handler (for embedding in a shared mux).
func (c *Channel) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(c.path, c.handleInbound)
	return mux
}

type inbound struct {
	sender    string // FromUserName, the allowlist key
	text      string
	id        string // MsgId for dedup
	mediaID   string // MediaId of an inbound image/voice message
	mediaType string // "image" | "audio"
}

func (c *Channel) handleInbound(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sig, timestamp, nonce := q.Get("msg_signature"), q.Get("timestamp"), q.Get("nonce")

	// GET: URL verification — decrypt echostr and echo the plaintext.
	if r.Method == http.MethodGet {
		echo := q.Get("echostr")
		if signature(c.cfg.Token, timestamp, nonce, echo) != sig {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		msg, _, err := c.decrypt(echo)
		if err != nil {
			http.Error(w, "bad echostr", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(msg)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	var env struct {
		Encrypt string `xml:"Encrypt"`
	}
	if err := xml.Unmarshal(body, &env); err != nil || env.Encrypt == "" {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if signature(c.cfg.Token, timestamp, nonce, env.Encrypt) != sig {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	plain, receiveID, err := c.decrypt(env.Encrypt)
	if err != nil {
		http.Error(w, "decrypt failed", http.StatusBadRequest)
		return
	}
	// The trailing receive_id must equal our corp id (WXBizMsgCrypt spec) — guards
	// against payloads encrypted for a different corp being replayed at us.
	if c.cfg.CorpID != "" && subtle.ConstantTimeCompare([]byte(receiveID), []byte(c.cfg.CorpID)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusOK) // WeCom accepts an empty 200; reply goes via the API.
	if m, ok := parseMessage(plain); ok {
		c.dispatch(r.Context(), m)
	}
}

func (c *Channel) dispatch(ctx context.Context, m inbound) {
	if m.sender == "" || (strings.TrimSpace(m.text) == "" && m.mediaID == "") {
		return
	}
	if m.id != "" && c.seenBefore(m.id) {
		return
	}
	msg := channel.UnifiedMessage{
		ChannelKind:  "wecom",
		ChannelID:    m.sender,
		Sender:       m.sender,
		Text:         m.text,
		PlatformTSMS: time.Now().UnixMilli(),
	}
	corr := "chan-" + ulid.New()
	allowed := c.cfg.Allowlist.Allows(m.sender)
	// Inbound media: fetch by media id (allowlisted senders only) so an image
	// reaches a vision model and a voice clip is transcribed.
	if allowed && m.mediaID != "" {
		if du := c.fetchMedia(ctx, m.mediaID, m.mediaType); du != "" {
			if m.mediaType == "audio" {
				msg.Audio = []string{du}
			} else {
				msg.Images = []string{du}
			}
		}
	}
	c.emitInbound(msg, corr, allowed)
	if !allowed || c.cfg.Handler == nil {
		return
	}
	rep, err := c.cfg.Handler(ctx, msg, corr)
	if err != nil {
		rep = channel.Reply{Text: "sorry — that failed: " + err.Error()}
	}
	reply := rep.Text
	if reply == "" {
		return
	}
	_ = c.Send(ctx, channel.Outbound{ChannelID: m.sender, Text: reply, Priority: channel.PriorityNotify})
}

// Send delivers out.Text to a WeCom user (out.ChannelID) via the app message API.
func (c *Channel) Send(ctx context.Context, out channel.Outbound) error {
	target := strings.TrimSpace(out.ChannelID)
	text := strings.TrimSpace(out.Text)
	if target == "" {
		return fmt.Errorf("wecom: send requires a user id")
	}
	if text == "" {
		return nil
	}
	tok, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	for _, chunk := range channel.SplitText(text, wecomMaxChars) {
		if err := c.sendOne(ctx, tok, target, chunk); err != nil {
			return err
		}
	}
	if c.cfg.Bus != nil {
		_, _ = c.cfg.Bus.Publish(event.Spec{
			Subject: "channel.outbound.wecom", Kind: event.KindChannelOutbound, Actor: "channel-wecom",
			Payload: map[string]any{"channel_kind": "wecom", "channel_id": target, "text": text},
		})
	}
	return nil
}

func (c *Channel) sendOne(ctx context.Context, token, user, text string) error {
	payload := map[string]any{
		"touser":  user,
		"msgtype": "text",
		"agentid": c.cfg.AgentID,
		"text":    map[string]string{"content": text},
	}
	raw, _ := json.Marshal(payload)
	url := c.apiBase + "/cgi-bin/message/send?access_token=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("wecom: send returned status %d", resp.StatusCode)
	}
	return nil
}

// fetchMedia downloads an inbound media file by id (cgi-bin/media/get) and
// returns it as an inline data: URL. Best-effort: returns "" on any failure.
func (c *Channel) fetchMedia(ctx context.Context, mediaID, mediaType string) string {
	if mediaID == "" {
		return ""
	}
	tok, err := c.accessToken(ctx)
	if err != nil || tok == "" {
		return ""
	}
	endpoint := c.apiBase + "/cgi-bin/media/get?access_token=" + url.QueryEscape(tok) + "&media_id=" + url.QueryEscape(mediaID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ""
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20+1))
	if err != nil || len(data) == 0 || len(data) > 16<<20 {
		return ""
	}
	// WeCom returns JSON ({"errcode":...}) on failure rather than binary.
	if len(data) > 0 && data[0] == '{' {
		return ""
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		if mediaType == "audio" {
			mime = "audio/amr"
		} else {
			mime = "image/jpeg"
		}
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func (c *Channel) accessToken(ctx context.Context) (string, error) {
	c.tmu.Lock()
	defer c.tmu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		return c.token, nil
	}
	url := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s", c.apiBase, c.cfg.CorpID, c.cfg.CorpSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var tr struct {
		ErrCode int    `json:"errcode"`
		Token   string `json:"access_token"`
		Exp     int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.Token == "" {
		return "", fmt.Errorf("wecom: token fetch failed (errcode %d)", tr.ErrCode)
	}
	c.token = tr.Token
	exp := tr.Exp
	if exp <= 0 {
		exp = 7200
	}
	c.tokenExp = time.Now().Add(time.Duration(exp-60) * time.Second)
	return c.token, nil
}

func (c *Channel) emitInbound(msg channel.UnifiedMessage, corr string, allowed bool) {
	if c.cfg.Bus == nil {
		return
	}
	_, _ = c.cfg.Bus.Publish(event.Spec{
		Subject:       "channel.inbound.wecom",
		Kind:          event.KindChannelInbound,
		Actor:         "channel-wecom",
		CorrelationID: corr,
		Payload: map[string]any{
			"channel_kind": "wecom", "channel_id": msg.ChannelID,
			"sender": msg.Sender, "text": msg.Text, "allowed": allowed,
		},
	})
}

func (c *Channel) seenBefore(id string) bool {
	c.dmu.Lock()
	defer c.dmu.Unlock()
	if _, ok := c.seen[id]; ok {
		return true
	}
	c.seen[id] = struct{}{}
	c.ring = append(c.ring, id)
	if len(c.ring) > dedupCapacity {
		old := c.ring[0]
		c.ring = c.ring[1:]
		delete(c.seen, old)
	}
	return false
}

// ---- WXBizMsgCrypt -------------------------------------------------------

// signature is the WeCom message signature: sha1 of the sorted concatenation of
// token, timestamp, nonce and the encrypted payload.
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
