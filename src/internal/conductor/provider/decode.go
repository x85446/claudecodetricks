package provider

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"io"
	"strings"
	"time"
)

const MaxRecordBytes = 1024 * 1024

// Decode drains JSONL incrementally. A malformed record emits a diagnostic and
// returns an error after draining, so malformed output cannot imply success.
func Decode(ctx context.Context, adapter string, r io.Reader, emit func(model.Event) error) error {
	reader := bufio.NewReaderSize(r, 64*1024)
	var bad error
	var session string
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var line []byte
		oversize := false
		for {
			chunk, more, e := reader.ReadLine()
			if e != nil {
				if e == io.EOF {
					return bad
				}
				return e
			}
			if len(line)+len(chunk) > MaxRecordBytes {
				oversize = true
			} else if !oversize {
				line = append(line, chunk...)
			}
			if !more {
				break
			}
		}
		if len(strings.TrimSpace(string(line))) == 0 && !oversize {
			continue
		}
		if oversize {
			bad = errors.New("provider record exceeds 1 MiB")
			if err := emit(model.Event{Version: 1, At: time.Now().UTC(), Kind: "diagnostic", Message: bad.Error()}); err != nil {
				return err
			}
			continue
		}
		var v map[string]json.RawMessage
		if err := json.Unmarshal(line, &v); err != nil {
			bad = errors.New("malformed provider JSON record")
			if err := emit(model.Event{Version: 1, At: time.Now().UTC(), Kind: "diagnostic", Message: bad.Error()}); err != nil {
				return err
			}
			continue
		}
		events, err := normalize(adapter, v)
		if err != nil {
			bad = err
			events = []model.Event{{Kind: "diagnostic", Message: err.Error()}}
		}
		for _, e := range events {
			e.Version = model.ProtocolVersion
			e.At = time.Now().UTC()
			if e.SessionID != "" {
				session = e.SessionID
			} else {
				e.SessionID = session
			}
			if err := emit(e); err != nil {
				return err
			}
		}
	}
}
func str(v map[string]json.RawMessage, k string) string {
	var s string
	_ = json.Unmarshal(v[k], &s)
	return s
}
func object(v map[string]json.RawMessage, k string) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(v[k], &m)
	return m
}
func number(v map[string]json.RawMessage, k string) int64 {
	var f float64
	_ = json.Unmarshal(v[k], &f)
	return int64(f)
}
func safeText(s string) string {
	if len(s) > 2048 {
		s = s[:2048] + "…"
	}
	return s
}
func normalize(adapter string, v map[string]json.RawMessage) ([]model.Event, error) {
	typ := str(v, "type")
	if typ == "" {
		return nil, errors.New("provider record has no type")
	}
	e := model.Event{ProviderID: str(v, "uuid"), SessionID: str(v, "session_id")}
	if adapter == "codex" {
		switch typ {
		case "thread.started":
			e.Kind = "session"
			e.SessionID = str(v, "thread_id")
		case "turn.started":
			e.Kind = "activity"
			e.Message = "Turn started"
		case "turn.completed":
			e.Kind = "terminal"
			e.Message = "Provider turn completed"
			u := object(v, "usage")
			e.InputTokens = number(u, "input_tokens")
			e.OutputTokens = number(u, "output_tokens")
			e.Cumulative = true
		case "turn.failed", "error":
			e.Kind = "error"
			e.Message = "Provider reported an error"
		case "item.started", "item.updated", "item.completed":
			item := object(v, "item")
			e.ProviderID = str(item, "id") + ":" + typ
			e.Kind = "activity"
			it := str(item, "type")
			e.Message = safeText(it + " " + strings.TrimPrefix(typ, "item."))
			if it == "command_execution" || it == "mcp_tool_call" || it == "web_search" || it == "file_change" {
				e.Kind = "tool"
			}
			if it == "agent_message" && typ == "item.completed" {
				if resultJSON(item["text"]) != nil {
					e.Kind = "result"
					e.Data = resultJSON(item["text"])
					e.Message = "Assignment result candidate"
				}
			}
		default:
			e.Kind = "diagnostic"
			e.Message = "Unknown Codex event: " + safeText(typ)
		}
	} else if adapter == "claude" || adapter == "claude-code" {
		switch typ {
		case "system":
			e.Kind = "session"
			e.Message = "Provider session initialized"
		case "assistant":
			e.Kind = "activity"
			e.Message = "Assistant message"
			msg := object(v, "message")
			u := object(msg, "usage")
			e.InputTokens = number(u, "input_tokens") + number(u, "cache_creation_input_tokens") + number(u, "cache_read_input_tokens")
			e.OutputTokens = number(u, "output_tokens")
			e.Cumulative = true
			var content []map[string]json.RawMessage
			_ = json.Unmarshal(msg["content"], &content)
			events := []model.Event{e}
			for _, c := range content {
				if str(c, "type") == "tool_use" {
					tool := model.Event{Kind: "tool", SessionID: e.SessionID, Message: "Tool: " + safeText(str(c, "name"))}
					if id := str(c, "id"); id != "" {
						tool.ProviderID = "tool:" + id + ":start"
					} else {
						tool.ProviderID = e.ProviderID + ":tool"
					}
					events = append(events, tool)
				}
			}
			return events, nil
		case "stream_event":
			e.Kind = "activity"
			ev := object(v, "event")
			e.Message = "Stream: " + safeText(str(ev, "type"))
			if str(ev, "type") == "content_block_start" {
				block := object(ev, "content_block")
				if str(block, "type") == "tool_use" && str(block, "id") != "" {
					e.Kind = "tool"
					e.ProviderID = "tool:" + str(block, "id") + ":start"
					e.Message = "Tool: " + safeText(str(block, "name"))
				}
			}
		case "user":
			e.Kind = "tool"
			e.Message = "Tool result received"
			msg := object(v, "message")
			var content []map[string]json.RawMessage
			_ = json.Unmarshal(msg["content"], &content)
			events := []model.Event{}
			for _, c := range content {
				if str(c, "type") == "tool_result" && str(c, "tool_use_id") != "" {
					tool := e
					tool.ProviderID = "tool:" + str(c, "tool_use_id") + ":end"
					events = append(events, tool)
				}
			}
			if len(events) > 0 {
				return events, nil
			}
		case "rate_limit_event":
			e.Kind = "allowance"
			e.Message = "Provider allowance observation"
			e.Data = v["rate_limit_info"]
		case "result":
			e.Kind = "terminal"
			e.Message = "Provider turn completed"
			var failed bool
			_ = json.Unmarshal(v["is_error"], &failed)
			if failed || str(v, "subtype") != "success" {
				e.Kind = "error"
				e.Message = "Provider turn failed"
			}
			u := object(v, "usage")
			e.InputTokens = number(u, "input_tokens") + number(u, "cache_creation_input_tokens") + number(u, "cache_read_input_tokens")
			e.OutputTokens = number(u, "output_tokens")
			e.Cumulative = true
			if e.Kind == "terminal" {
				data := resultJSON(v["result"])
				if data == nil {
					data = resultJSON(v["structured_output"])
				}
				if data != nil {
					return []model.Event{e, {Kind: "result", SessionID: e.SessionID, ProviderID: e.ProviderID + ":result", Message: "Assignment result candidate", Data: data}}, nil
				}
			}
		default:
			e.Kind = "diagnostic"
			e.Message = "Unknown Claude event: " + safeText(typ)
		}
	} else if adapter == "jsonl" {
		// The generic JSONL contract accepts only normalized, bounded public fields.
		if err := json.Unmarshal(mustMarshal(v), &e); err != nil {
			return nil, fmt.Errorf("invalid normalized event")
		}
		switch e.Kind {
		case "session", "activity", "tool", "usage", "error", "terminal", "result", "diagnostic":
		default:
			return nil, fmt.Errorf("unsupported normalized event kind")
		}
		e.Message = safeText(e.Message)
		if e.Kind != "result" {
			e.Data = nil
		}
	} else {
		return []model.Event{{Kind: "diagnostic", Message: "Structured stream unsupported for adapter"}}, nil
	}
	return []model.Event{e}, nil
}
func mustMarshal(v any) []byte { b, _ := json.Marshal(v); return b }
func resultJSON(raw json.RawMessage) json.RawMessage {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		raw = []byte(text)
	}
	var r model.Result
	if json.Unmarshal(raw, &r) != nil || r.Version != model.ProtocolVersion || r.AssignmentID == "" {
		return nil
	}
	return mustMarshal(r)
}

// ExtractResult extracts a candidate only; the supervisor validates identity,
// evidence and source digest before persistence or advancement.
func ExtractResult(e model.Event) (model.Result, error) {
	var r model.Result
	if e.Kind != "result" {
		return r, errors.New("event is not an assignment result")
	}
	err := json.Unmarshal(e.Data, &r)
	return r, err
}
