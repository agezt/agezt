// SPDX-License-Identifier: MIT

// Command idechoplugin is a minimal protocol fixture for the M180
// reload test: its single tool ("idecho") returns, as Output, the
// host-assigned request id it received. That lets the test observe the
// host's correlation-id sequence across a Reload and assert the ids
// keep climbing (never reset / reused).
package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type frame struct {
	ID     string          `json:"id"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

func main() {
	dec := bufio.NewReader(os.Stdin)
	enc := bufio.NewWriter(os.Stdout)
	defer enc.Flush()

	write := func(r response) {
		raw, _ := json.Marshal(r)
		_, _ = enc.Write(raw)
		_ = enc.WriteByte('\n')
		_ = enc.Flush()
	}

	for {
		line, err := dec.ReadBytes('\n')
		if err != nil {
			return
		}
		var f frame
		if err := json.Unmarshal(line, &f); err != nil {
			continue
		}
		switch f.Method {
		case "initialize":
			defs := []toolDef{{
				Name:        "idecho",
				Description: "Returns the host-assigned request id as Output.",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			}}
			raw, _ := json.Marshal(struct {
				Tools []toolDef `json:"tools"`
			}{Tools: defs})
			write(response{ID: f.ID, Result: raw})
		case "tool/invoke":
			// Echo the id the host assigned to THIS request.
			raw, _ := json.Marshal(struct {
				Output string `json:"output"`
			}{Output: f.ID})
			write(response{ID: f.ID, Result: raw})
		case "shutdown":
			return
		default:
			write(response{ID: f.ID, Error: "unknown method: " + f.Method})
		}
	}
}
