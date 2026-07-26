// SPDX-License-Identifier: MIT

package stt

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("broken body") }
func (failingBody) Close() error             { return nil }

func TestNew_Defaults(t *testing.T) {
	c := New(Config{})
	if c.base != "https://api.openai.com/v1" {
		t.Errorf("base = %q, want OpenAI default", c.base)
	}
	if c.Model() != "whisper-1" {
		t.Errorf("model = %q, want whisper-1", c.Model())
	}
	c2 := New(Config{APIURL: "http://127.0.0.1:9000/v1/", Model: "whisper-large"})
	if c2.base != "http://127.0.0.1:9000/v1" || c2.Model() != "whisper-large" {
		t.Errorf("overrides not applied: base=%q model=%q", c2.base, c2.Model())
	}
}

// Transcribe posts a multipart body carrying the audio, the filename, the model,
// and the bearer token, and returns the server's text.
func TestTranscribe_PostsMultipartAndReturnsText(t *testing.T) {
	var gotAuth, gotModel, gotFilename, gotAudio, gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		gotModel = r.FormValue("model")
		f, hdr, err := r.FormFile("file")
		if err == nil {
			gotFilename = hdr.Filename
			b, _ := io.ReadAll(f)
			gotAudio = string(b)
		}
		_, _ = io.WriteString(w, `{"text":"  hello world  "}`)
	}))
	defer srv.Close()

	c := New(Config{APIURL: srv.URL, APIKey: "sk-test", Model: "whisper-1", HTTPClient: srv.Client()})
	text, err := c.Transcribe(context.Background(), "clip.wav", []byte("RIFFfake-wav-bytes"))
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want trimmed 'hello world'", text)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth = %q", gotAuth)
	}
	if gotModel != "whisper-1" {
		t.Errorf("model field = %q", gotModel)
	}
	if gotFilename != "clip.wav" {
		t.Errorf("filename = %q", gotFilename)
	}
	if gotAudio != "RIFFfake-wav-bytes" {
		t.Errorf("audio = %q", gotAudio)
	}
	if !strings.HasPrefix(gotContentType, "multipart/form-data") {
		t.Errorf("content-type = %q, want multipart", gotContentType)
	}
}

func TestTranscribe_EmptyAudioErrors(t *testing.T) {
	c := New(Config{APIURL: "http://127.0.0.1:1", APIKey: "k"})
	if _, err := c.Transcribe(context.Background(), "x.wav", nil); err == nil {
		t.Error("empty audio must error before any request")
	}
}

func TestTranscribe_HTTPErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()
	c := New(Config{APIURL: srv.URL, APIKey: "k", HTTPClient: srv.Client()})
	_, err := c.Transcribe(context.Background(), "x.wav", []byte("audio"))
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("want a 401 error, got %v", err)
	}
}

// A 200 with an error body (some gateways do this) surfaces the message.
func TestTranscribe_BodyErrorSurfaced(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"error":{"message":"model unavailable"}}`)
	}))
	defer srv.Close()
	c := New(Config{APIURL: srv.URL, HTTPClient: srv.Client()})
	_, err := c.Transcribe(context.Background(), "x.wav", []byte("audio"))
	if err == nil || !strings.Contains(err.Error(), "model unavailable") {
		t.Errorf("want a body-error message, got %v", err)
	}
}

func TestTranscribe_BadURLAndTransportErrors(t *testing.T) {
	c := New(Config{APIURL: "http://bad\x7fhost", APIKey: "secret"})
	if _, err := c.Transcribe(context.Background(), "x.wav", []byte("audio")); err == nil {
		t.Fatal("invalid request URL must fail")
	}

	c = New(Config{
		APIURL: "http://voice.test/v1",
		APIKey: "secret",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("transport exposed secret")
		})},
	})
	_, err := c.Transcribe(context.Background(), "x.wav", []byte("audio"))
	if err == nil || strings.Contains(err.Error(), "secret") || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("transport error was not safely surfaced: %v", err)
	}
}

func TestTranscribe_ResponseReadDecodeAndSizeErrors(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		c := New(Config{
			APIURL: "http://voice.test/v1",
			HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: failingBody{}}, nil
			})},
		})
		if _, err := c.Transcribe(context.Background(), "x.wav", []byte("audio")); err == nil ||
			!strings.Contains(err.Error(), "read response") {
			t.Fatalf("expected read error, got %v", err)
		}
	})

	t.Run("decode", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "{bad json")
		}))
		defer srv.Close()
		c := New(Config{APIURL: srv.URL, HTTPClient: srv.Client()})
		if _, err := c.Transcribe(context.Background(), "x.wav", []byte("audio")); err == nil ||
			!strings.Contains(err.Error(), "decode response") {
			t.Fatalf("expected decode error, got %v", err)
		}
	})

	t.Run("too large", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", sttRespMaxBytes+1))
		}))
		defer srv.Close()
		c := New(Config{APIURL: srv.URL, HTTPClient: srv.Client()})
		if _, err := c.Transcribe(context.Background(), "x.wav", []byte("audio")); err == nil ||
			!strings.Contains(err.Error(), "response exceeds") {
			t.Fatalf("expected response-size error, got %v", err)
		}
	})
}

func TestScrub(t *testing.T) {
	plain := errors.New("plain failure")
	if got := (&Client{}).scrub(nil); got != nil {
		t.Fatalf("scrub(nil) = %v", got)
	}
	if got := (&Client{}).scrub(plain); got != plain {
		t.Fatal("client without a key should preserve the error")
	}
	if got := (&Client{key: "secret"}).scrub(plain); got != plain {
		t.Fatal("error without the key should be preserved")
	}
	got := (&Client{key: "secret"}).scrub(errors.New("secret leaked twice: secret"))
	if strings.Contains(got.Error(), "secret") || strings.Count(got.Error(), "<redacted>") != 2 {
		t.Fatalf("key was not fully redacted: %v", got)
	}
}
