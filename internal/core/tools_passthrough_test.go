package core

// 网关出站组包的 tools 透传单测：验证 chatRequest 的 tools/tool_choice 与
// 第二轮消息形态（assistant.tool_calls / tool 结果）完整进入上游请求体。

import (
	"encoding/json"
	"testing"
)

func TestUpstreamChatRequestToolsPassthrough(t *testing.T) {
	req := chatRequest{
		Model: "glm-5.2",
		Messages: []ChatMessage{
			{Role: "user", Content: "北京天气"},
			{Role: "assistant", Content: "",
				ToolCalls: json.RawMessage(`[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"北京\"}"}}]`)},
			{Role: "tool", Content: `{"temp":"12℃"}`, ToolCallID: "call_1", Name: "get_weather"},
		},
		Tools:      json.RawMessage(`[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object"}}}]`),
		ToolChoice: json.RawMessage(`"auto"`),
	}
	raw := upstreamChatRequest(req)
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("组包不是合法 JSON: %v", err)
	}
	// tools / tool_choice 透传
	if _, ok := out["tools"]; !ok {
		t.Fatal("tools 字段丢失")
	}
	if out["tool_choice"] != "auto" {
		t.Fatalf("tool_choice 透传错误: %v", out["tool_choice"])
	}
	// 第二轮消息形态
	msgs, _ := out["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("messages 数量错误: %d", len(msgs))
	}
	asst, _ := msgs[1].(map[string]any)
	if _, ok := asst["tool_calls"]; !ok {
		t.Fatal("assistant.tool_calls 丢失")
	}
	tool, _ := msgs[2].(map[string]any)
	if tool["tool_call_id"] != "call_1" {
		t.Fatalf("tool.tool_call_id 丢失: %v", tool["tool_call_id"])
	}
	if tool["role"] != "tool" {
		t.Fatalf("tool 角色丢失: %v", tool["role"])
	}
}

func TestUpstreamChatRequestNoToolsUnchanged(t *testing.T) {
	// 无 tools 请求组包不应带 tools 键（回归保护：普通聊天/签到链路形态不变）
	req := chatRequest{
		Model:    "glm-5.2",
		Messages: []ChatMessage{{Role: "user", Content: "hi"}},
	}
	raw := upstreamChatRequest(req)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if _, ok := out["tools"]; ok {
		t.Fatal("无 tools 请求不应携带 tools 字段")
	}
}
