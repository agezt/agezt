// SPDX-License-Identifier: MIT
//
// cmd/agt `ha call` verb + shared HTTP request helper + CLI constants.
// Extracted from ha.go during Day 211 god-file refactor (#75).
// Public API unchanged.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

const (
	haCLITimeout = 30 * time.Second
	haMaxBody    = 1 << 20 // cap the response we read for display (1 MiB)
)

func haCall(base, token string, args []string, stdout, stderr io.Writer) int {
	var target, entity, data string
	raw := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			raw = true
		case a == "--entity":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s ha call: --entity needs a value\n", brand.CLI)
				return 2
			}
			i++
			entity = args[i]
		case strings.HasPrefix(a, "--entity="):
			entity = strings.TrimPrefix(a, "--entity=")
		case a == "--data":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s ha call: --data needs a value\n", brand.CLI)
				return 2
			}
			i++
			data = args[i]
		case strings.HasPrefix(a, "--data="):
			data = strings.TrimPrefix(a, "--data=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s ha call: unknown flag %q\n", brand.CLI, a)
			return 2
		case target == "":
			target = a
		default:
			fmt.Fprintf(stderr, "%s ha call: unexpected argument %q\n", brand.CLI, a)
			return 2
		}
	}
	domain, service, ok := strings.Cut(target, ".")
	if !ok || domain == "" || service == "" {
		fmt.Fprintf(stderr, "usage: %s ha call <domain.service> [--entity id] [--data json]\n", brand.CLI)
		return 2
	}

	payload := map[string]any{}
	if strings.TrimSpace(data) != "" {
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			fmt.Fprintf(stderr, "%s ha call: --data is not valid JSON: %v\n", brand.CLI, err)
			return 2
		}
	}
	if entity != "" {
		payload["entity_id"] = entity
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s ha call: %v\n", brand.CLI, err)
		return 1
	}

	path := "/api/services/" + url.PathEscape(domain) + "/" + url.PathEscape(service)
	status, body, err := haRequest(http.MethodPost, base, token, path, enc)
	if code := haCheck(status, body, err, stderr); code != 0 {
		return code
	}
	fmt.Fprintf(stdout, "called %s.%s ok\n", domain, service)
	return printBodyJSON(body, raw, stdout)
}
func haRequest(method, base, token, path string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), haCLITimeout)
	defer cancel()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, haMaxBody+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(out) > haMaxBody {
		out = out[:haMaxBody]
	}
	return resp.StatusCode, out, nil
}
