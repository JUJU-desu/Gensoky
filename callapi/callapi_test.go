package callapi

import (
	"encoding/json"
	"testing"
)

func TestUnmarshalActionMessage_RequestID(t *testing.T) {
	cases := []struct{
		name string
		jsonStr string
		expected string
		hasEcho bool
	}{
		{
			name: "top-level string request_id",
			jsonStr: `{ "action": "send_group_msg", "params": { "group_id": 123 }, "request_id": "abc123" }`,
			expected: "abc123",
			hasEcho: true,
		},
		{
			name: "params string request_id",
			jsonStr: `{ "action": "send_group_msg", "params": { "group_id": 123, "request_id": "xyz789" } }`,
			expected: "xyz789",
			hasEcho: true,
		},
		{
			name: "params numeric request_id",
			jsonStr: `{ "action": "send_group_msg", "params": { "group_id": 123, "request_id": 987654 } }`,
			expected: "987654",
			hasEcho: true,
		},
		{
			name: "only echo",
			jsonStr: `{ "action": "get_status", "params": {}, "echo": "abcEcho" }`,
			expected: "abcEcho",
			hasEcho: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var msg ActionMessage
			if err := json.Unmarshal([]byte(c.jsonStr), &msg); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			// Debug: ensure params contains request_id when expected
			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(c.jsonStr), &raw); err == nil {
				if p, ok := raw["params"]; ok {
					var pm map[string]json.RawMessage
					if err := json.Unmarshal(p, &pm); err == nil {
						if _, ok := pm["request_id"]; ok {
							t.Logf("params contains request_id")
						}
						if _, ok := pm["requestID"]; ok {
							t.Logf("params contains requestID")
						}
					}
				}
			}
			if c.hasEcho {
				if msg.RequestID == nil {
					t.Fatalf("expected RequestID to be set, got nil, full: %+v", msg)
				}
				// compare string representation
				s := ""
				switch v := msg.RequestID.(type) {
				case string:
					s = v
				default:
					t.Fatalf("expected RequestID to be string, got %T", v)
				}
				if s != c.expected {
					t.Fatalf("expected %s, got %s", c.expected, s)
				}

				// Also ensure Params.RequestID is populated when available
				if msg.Params.RequestID == nil {
					t.Fatalf("expected Params.RequestID to be set, got nil, full: %+v", msg)
				}
				pstr := ""
				switch v := msg.Params.RequestID.(type) {
				case string:
					pstr = v
				default:
					t.Fatalf("expected Params.RequestID to be string, got %T", v)
				}
				if pstr != c.expected {
					t.Fatalf("expected Params.RequestID %s, got %s", c.expected, pstr)
				}
			}
		})
	}
}
