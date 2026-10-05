// SPDX-License-Identifier: MIT

// Package board owns transport-independent message-board use cases.
package board

import (
	"context"
	"errors"
	"fmt"
	shared "github.com/agezt/agezt/kernel/board"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Store interface {
	Read(string, int) []shared.Message
	Topics() map[string]int
	OpenHelp(int) []shared.Message
	Get(string) (shared.Message, bool)
	Inbox(string, int, bool) []shared.Message
	Replies(string, int) []shared.Message
	Post(string, string, string, int64) (shared.Message, error)
	Send(shared.Message, int64) (shared.Message, error)
	HelpRequest(string, string, string, int64) (shared.Message, error)
	Broadcast(string, string, int64) (shared.Message, error)
	Ack(string, string) (shared.Message, bool, error)
}
type Service struct {
	store  Store
	notify func(shared.Message, string)
}

// The host selects its shared writer or read-only fallback before construction.
func New(store Store, notify func(shared.Message, string)) *Service {
	return &Service{store: store, notify: notify}
}

type Message struct {
	Topic    string   `json:"topic"`
	Text     string   `json:"text"`
	TSUnixMS int64    `json:"ts_unix_ms"`
	ID       string   `json:"id,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	ReplyTo  string   `json:"reply_to,omitempty"`
	Help     bool     `json:"help,omitempty"`
	AckedBy  []string `json:"acked_by,omitempty"`
}

func project(m shared.Message) Message {
	return Message{Topic: m.Topic, Text: m.Text, TSUnixMS: m.TSMS, ID: m.ID, From: m.From, To: m.To, ReplyTo: m.ReplyTo, Help: m.Help, AckedBy: append([]string(nil), m.AckedBy...)}
}
func projectAll(messages []shared.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		out = append(out, project(m))
	}
	return out
}

type ReadInput struct {
	Topic  string `json:"topic,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type ReadOutput struct {
	Messages   []Message      `json:"messages"`
	Topics     map[string]int `json:"topics"`
	Count      int            `json:"count"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

func (s *Service) Read(_ context.Context, in ReadInput) (ReadOutput, error) {
	var cursorTS int64
	var cursorID string
	cursorOK := false
	if in.Cursor != "" {
		tsStr, id, _ := strings.Cut(in.Cursor, ":")
		if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
			cursorTS, cursorID, cursorOK = ts, id, true
		}
	}
	messages := s.store.Read(in.Topic, 0)
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].TSMS != messages[j].TSMS {
			return messages[i].TSMS > messages[j].TSMS
		}
		return messages[i].ID > messages[j].ID
	})
	total := len(messages)
	if cursorOK {
		filtered := messages[:0]
		for _, m := range messages {
			if m.TSMS > cursorTS {
				continue
			}
			if m.TSMS == cursorTS && m.ID >= cursorID {
				continue
			}
			filtered = append(filtered, m)
		}
		messages = filtered
	}
	var next string
	if in.Limit > 0 && len(messages) > in.Limit {
		messages = messages[:in.Limit]
		last := messages[in.Limit-1]
		next = strconv.FormatInt(last.TSMS, 10) + ":" + last.ID
	}
	views := projectAll(messages)
	return ReadOutput{Messages: views, Topics: s.store.Topics(), Count: len(views), Total: total, NextCursor: next}, nil
}

type LimitInput struct {
	Limit int `json:"limit,omitempty"`
}
type HelpOutput struct {
	OpenHelp []Message `json:"open_help"`
	Count    int       `json:"count"`
}

func (s *Service) Help(_ context.Context, in LimitInput) (HelpOutput, error) {
	views := projectAll(s.store.OpenHelp(in.Limit))
	return HelpOutput{OpenHelp: views, Count: len(views)}, nil
}

type InboxInput struct {
	To    string `json:"to"`
	Limit int    `json:"limit,omitempty"`
	All   bool   `json:"all,omitempty"`
}
type InboxOutput struct {
	To      string    `json:"to"`
	Waiting []Message `json:"waiting"`
	Count   int       `json:"count"`
}

func (s *Service) Inbox(_ context.Context, in InboxInput) (InboxOutput, error) {
	if in.To == "" {
		return InboxOutput{}, errors.New("board_inbox requires to (whose inbox)")
	}
	views := projectAll(s.store.Inbox(in.To, in.Limit, in.All))
	return InboxOutput{To: in.To, Waiting: views, Count: len(views)}, nil
}

type GetInput struct {
	ID string `json:"id"`
}
type GetOutput struct {
	Message Message `json:"message"`
}

func (s *Service) Get(_ context.Context, in GetInput) (GetOutput, error) {
	if in.ID == "" {
		return GetOutput{}, errors.New("board_get requires id")
	}
	m, found := s.store.Get(in.ID)
	if !found {
		return GetOutput{}, fmt.Errorf("no message with id %s", in.ID)
	}
	return GetOutput{Message: project(m)}, nil
}

type RepliesInput struct {
	ID    string `json:"id"`
	Limit int    `json:"limit,omitempty"`
}
type RepliesOutput struct {
	ID      string    `json:"id"`
	Replies []Message `json:"replies"`
	Count   int       `json:"count"`
}

func (s *Service) Replies(_ context.Context, in RepliesInput) (RepliesOutput, error) {
	if in.ID == "" {
		return RepliesOutput{}, errors.New("board_replies requires id")
	}
	views := projectAll(s.store.Replies(in.ID, in.Limit))
	return RepliesOutput{ID: in.ID, Replies: views, Count: len(views)}, nil
}

type AckInput struct {
	ID string `json:"id"`
	By string `json:"by"`
}
type AckOutput struct {
	Acked bool   `json:"acked"`
	ID    string `json:"id"`
	By    string `json:"by"`
}

func (s *Service) Ack(_ context.Context, in AckInput) (AckOutput, error) {
	if in.ID == "" || in.By == "" {
		return AckOutput{}, errors.New("board_ack requires id and by")
	}
	_, found, err := s.store.Ack(in.ID, in.By)
	if err != nil {
		return AckOutput{}, err
	}
	if !found {
		return AckOutput{}, fmt.Errorf("no message with id %s", in.ID)
	}
	return AckOutput{Acked: true, ID: in.ID, By: in.By}, nil
}

type SendInput struct {
	Text          string `json:"text"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
	Topic         string `json:"topic,omitempty"`
	ReplyTo       string `json:"reply_to,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Help          bool   `json:"help,omitempty"`
}
type SendOutput struct {
	Sent          Message `json:"sent"`
	CorrelationID string  `json:"correlation_id,omitempty"`
}

func (s *Service) Send(_ context.Context, in SendInput) (SendOutput, error) {
	if in.Text == "" {
		return SendOutput{}, errors.New("board_send requires text")
	}
	now := time.Now().UnixMilli()
	var m shared.Message
	var err error
	switch {
	case in.ReplyTo != "":
		orig, found := s.store.Get(in.ReplyTo)
		if !found {
			return SendOutput{}, fmt.Errorf("no message with id %s", in.ReplyTo)
		}
		m, err = s.store.Send(shared.Message{Topic: orig.Topic, From: in.From, To: orig.From, ReplyTo: orig.ID, Text: in.Text}, now)
	case in.Help:
		m, err = s.store.HelpRequest(in.From, in.To, in.Text, now)
	case in.To == shared.Everyone:
		m, err = s.store.Broadcast(in.From, in.Text, now)
	case in.To != "":
		topic := in.Topic
		if topic == "" {
			topic = "dm"
		}
		m, err = s.store.Send(shared.Message{Topic: topic, From: in.From, To: in.To, Text: in.Text}, now)
	default:
		if in.Topic == "" {
			return SendOutput{}, errors.New("board_send requires a topic (for a post) or a to (for a DM / \"*\" broadcast)")
		}
		m, err = s.store.Post(in.Topic, in.From, in.Text, now)
	}
	if err != nil {
		return SendOutput{}, err
	}
	if s.notify != nil {
		s.notify(m, in.CorrelationID)
	}
	return SendOutput{Sent: project(m), CorrelationID: in.CorrelationID}, nil
}
