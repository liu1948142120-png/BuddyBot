package core

// 上游 tools（function calling）支持探针（A/B 对照版）。
// A 组：复刻网关出站形态的裸请求（无 tools）——验证指纹与凭证可用（基线）；
// B 组：同一请求 + tools/tool_choice——若 A 过而 B 被 400+11128 拦，
//       则结论为上游 WAF 对 tools 字段拦截，网关透传也无法支持工具调用。
// 仅本地诊断用：go test -run TestUpstreamToolsProbe -v

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func probeCreds(t *testing.T) []*UpstreamCred {
	t.Helper()
	dir := os.Getenv("BB_AUTH_DIR")
	if dir == "" {
		dir = `E:\WorkBuddy积分签到\BuddyBot_Portable_v9\.buddybot\auths`
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读 auths 目录失败: %v", err)
	}
	var out []*UpstreamCred
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		cred, err := LoadUpstreamCred(dir, e.Name())
		if err != nil {
			t.Logf("跳过凭证 %s: %v", e.Name(), err)
			continue
		}
		out = append(out, cred)
	}
	if len(out) == 0 {
		t.Fatal("无可用凭证")
	}
	return out
}

// streamOnce 发送 body 并聚合 SSE：返回 (文本, toolCalls 是否出现, 状态码, 错误体)。
func streamOnce(t *testing.T, c *upstreamClient, cred *UpstreamCred, raw []byte) (string, bool, int, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	res, err := c.chat(ctx, cred.Realm, cred, raw)
	if err != nil {
		t.Fatalf("上游请求失败: %v", err)
	}
	if res.status >= 400 {
		return "", false, res.status, res.errBody
	}
	defer res.body.Close()
	var text strings.Builder
	sawTools := false
	sc := bufio.NewScanner(res.body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var chunk upstreamChunk
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			for _, tc := range ch.Delta.ToolCalls {
				sawTools = true
				t.Logf("  TOOL_CALL: name=%s args=%s", tc.Function.Name, tc.Function.Arguments)
			}
			text.WriteString(ch.Delta.Content)
		}
	}
	return text.String(), sawTools, res.status, nil
}

func TestUpstreamToolsProbe(t *testing.T) {
	creds := probeCreds(t)
	c := newUpstreamClient()
	model := os.Getenv("BB_PROBE_MODEL")
	if model == "" {
		model = "glm-5.2"
	}

	// ---- A 组：基线（无 tools，形态与网关出站一致，含 prompt_cache_key）----
	// 遍历凭证直到某个基线通过（指纹可用），再以同一凭证做 B 组对照。
	base := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": "回复两个字：在线"},
		},
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	var cred *UpstreamCred
	for i, cr := range creds {
		rawA0, _ := json.Marshal(base)
		rawA := injectPromptCacheKey(rawA0, cr.UID, "probe-session")
		text, saw, st, errBody := streamOnce(t, c, cr, rawA)
		if st >= 400 {
			t.Logf("凭证#%d(%s) 基线 %d: %s", i, cr.Realm, st, truncateBody(errBody))
			continue
		}
		t.Logf("凭证#%d(%s) 基线通过: text=%q tool_calls=%v", i, cr.Realm, truncateBody([]byte(text)), saw)
		cred = cr
		break
	}
	if cred == nil {
		t.Fatal("全部凭证基线均失败——直连指纹问题，B组无需进行；网关链路不受此影响")
	}

	// ---- B 组：同请求 + tools（auto）----
	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "查询指定城市的实时天气",
				"parameters": map[string]any{
					"type":       "object",
					"properties": map[string]any{"city": map[string]any{"type": "string", "description": "城市名"}},
					"required":   []string{"city"},
				},
			},
		},
	}
	b := map[string]any{
		"model": model,
		"messages": []map[string]any{
			{"role": "user", "content": "北京今天天气怎么样？必须调用工具查询。"},
		},
		"tools":          tools,
		"tool_choice":    "auto",
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	rawB0, _ := json.Marshal(b)
	rawB := injectPromptCacheKey(rawB0, cred.UID, "probe-session")
	textB, sawB, stB, errBodyB := streamOnce(t, c, cred, rawB)
	if stB >= 400 {
		t.Logf("B组(tools): status=%d err=%s", stB, truncateBody(errBodyB))
		if IsFingerprintBlock(stB, string(errBodyB)) {
			t.Logf("== 结论：基线可过、带 tools 被 11128 指纹拦截 → 上游 WAF 拒绝 tools 字段，工具调用不可行 ==")
		} else {
			t.Logf("== 结论：上游以 %d 拒绝 tools 请求（非指纹类）==", stB)
		}
		return
	}
	t.Logf("B组(tools): status=%d text=%q tool_calls=%v", stB, truncateBody([]byte(textB)), sawB)
	if sawB {
		t.Logf("== 结论：上游完整支持 OpenAI tools，模型返回 tool_calls → 网关透传即可支持工具调用 ==")
	} else {
		t.Logf("== 结论：上游接受 tools 字段（未报错）但模型未发起调用，需进一步验证 ==")
	}
}

// TestUpstreamToolRoundTrip 验证工具调用第二轮：assistant(tool_calls) + tool(结果) 消息形态上游是否接受。
func TestUpstreamToolRoundTrip(t *testing.T) {
	var cred *UpstreamCred
	for _, cr := range probeCreds(t) {
		if cr.Realm == "cn" {
			cred = cr
			break
		}
	}
	if cred == nil {
		t.Skip("无 cn 凭证")
	}
	c := newUpstreamClient()
	tools := []map[string]any{
		{"type": "function", "function": map[string]any{
			"name":        "get_weather",
			"description": "查询指定城市的实时天气",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"city": map[string]any{"type": "string"}},
				"required":   []string{"city"},
			},
		}},
	}
	// 轮1：拿真实 tool_calls（id/name/args）
	msgs1 := []map[string]any{{"role": "user", "content": "北京天气？调用工具。"}}
	b1 := map[string]any{"model": "glm-5.2", "messages": msgs1, "tools": tools, "tool_choice": "auto",
		"stream": true, "stream_options": map[string]any{"include_usage": true}}
	raw1, _ := json.Marshal(b1)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	res, err := c.chat(ctx, cred.Realm, cred, raw1)
	if err != nil {
		t.Fatalf("轮1失败: %v", err)
	}
	if res.status >= 400 {
		t.Fatalf("轮1 上游 %d: %s", res.status, truncateBody(res.errBody))
	}
	type tcAcc struct{ id, name, args strings.Builder }
	accs := map[int]*tcAcc{}
	sc := bufio.NewScanner(res.body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if p == "" || p == "[DONE]" {
			continue
		}
		var ch upstreamChunk
		if json.Unmarshal([]byte(p), &ch) != nil {
			continue
		}
		for _, choice := range ch.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				a := accs[tc.Index]
				if a == nil {
					a = &tcAcc{}
					accs[tc.Index] = a
				}
				if tc.ID != "" {
					a.id.WriteString(tc.ID)
				}
				if tc.Function.Name != "" {
					a.name.WriteString(tc.Function.Name)
				}
				a.args.WriteString(tc.Function.Arguments)
			}
		}
	}
	res.body.Close()
	if len(accs) == 0 {
		t.Fatal("轮1 未收到 tool_calls")
	}
	// 取第一个聚合结果
	var idx int
	for k := range accs {
		idx = k
	}
	ac := accs[idx]
	toolCallID := ac.id.String()
	if toolCallID == "" {
		toolCallID = "call_probe001"
	}
	t.Logf("轮1 tool_call: id=%s name=%s args=%s", toolCallID, ac.name.String(), ac.args.String())

	// 轮2：assistant(tool_calls) + tool(result) —— OpenAI 标准形态
	msgs2 := []map[string]any{
		{"role": "user", "content": "北京天气？调用工具。"},
		{"role": "assistant", "content": "", "tool_calls": []map[string]any{
			{"id": toolCallID, "type": "function",
				"function": map[string]any{"name": ac.name.String(), "arguments": ac.args.String()}},
		}},
		{"role": "tool", "tool_call_id": toolCallID, "content": `{"city":"北京","temp":"12℃","cond":"晴"}`},
	}
	b2 := map[string]any{"model": "glm-5.2", "messages": msgs2, "tools": tools, "tool_choice": "auto",
		"stream": true, "stream_options": map[string]any{"include_usage": true}}
	raw2, _ := json.Marshal(b2)
	text, saw, st, errBody := streamOnce(t, c, cred, raw2)
	if st >= 400 {
		t.Logf("轮2 上游 %d: %s", st, truncateBody(errBody))
		t.Log("== 结论：上游不接受标准第二轮形态，需要适配 ==")
		return
	}
	t.Logf("轮2: status=%d tool_calls=%v text=%q", st, saw, truncateBody([]byte(text)))
	t.Log("== 结论：上游接受 OpenAI 标准两轮工具调用形态 ==")
}
