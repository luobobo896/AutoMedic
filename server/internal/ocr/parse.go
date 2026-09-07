package ocr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Finding 归一化后的审查意见（OCR JSON 字段名不统一，解析时做兼容）
type Finding struct {
	Key      string `json:"key"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	EndLine  int    `json:"end_line,omitempty"`
	Severity string `json:"severity"`
	Rule     string `json:"rule,omitempty"`
	Title    string `json:"title"`
	Body     string `json:"body,omitempty"`
}

// ParseFindings 从 ocr --format json 的文件或 stdout 提取意见。
// 兼容 comments / findings / issues / results，以及嵌套 data。
func ParseFindings(raw []byte) ([]Finding, error) {
	raw = trimJSON(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("OCR 输出为空")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		if extracted := extractJSON(raw); len(extracted) > 0 {
			if err2 := json.Unmarshal(extracted, &v); err2 != nil {
				return nil, fmt.Errorf("解析 OCR JSON 失败: %w", err)
			}
		} else {
			return nil, fmt.Errorf("解析 OCR JSON 失败: %w", err)
		}
	}
	items := collectItems(v)
	out := make([]Finding, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		f, ok := mapFinding(item)
		if !ok {
			continue
		}
		if seen[f.Key] {
			continue
		}
		seen[f.Key] = true
		out = append(out, f)
	}
	return out, nil
}

func trimJSON(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	return []byte(s)
}

func extractJSON(b []byte) []byte {
	s := string(b)
	i := strings.IndexAny(s, "{[")
	if i < 0 {
		return nil
	}
	return []byte(strings.TrimSpace(s[i:]))
}

func collectItems(v any) []map[string]any {
	switch t := v.(type) {
	case []any:
		return mapsFromArray(t)
	case map[string]any:
		for _, k := range []string{"comments", "findings", "issues", "results", "items"} {
			if arr, ok := t[k].([]any); ok {
				return mapsFromArray(arr)
			}
		}
		if data, ok := t["data"]; ok {
			return collectItems(data)
		}
		if looksLikeFinding(t) {
			return []map[string]any{t}
		}
	}
	return nil
}

func mapsFromArray(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func looksLikeFinding(m map[string]any) bool {
	return firstString(m, "file", "path", "filename", "filePath", "file_path") != "" ||
		firstString(m, "title", "message", "body", "comment", "content", "summary") != ""
}

func mapFinding(m map[string]any) (Finding, bool) {
	path := firstString(m, "file", "path", "filename", "filePath", "file_path", "relativePath")
	title := firstString(m, "title", "message", "summary", "comment", "body", "content", "text")
	body := firstString(m, "body", "content", "comment", "message", "description", "suggestion", "advice")
	if path == "" && title == "" {
		return Finding{}, false
	}
	line := firstInt(m, "line", "startLine", "start_line", "lineNumber", "line_number", "beginLine")
	end := firstInt(m, "endLine", "end_line", "end")
	sev := strings.ToLower(firstString(m, "severity", "level", "priority", "rank"))
	sev = normalizeSeverity(sev)
	rule := firstString(m, "rule", "ruleId", "rule_id", "category", "checker", "id")
	if title == "" {
		title = body
	}
	if title == "" {
		title = path
	}
	f := Finding{
		Path:     strings.TrimSpace(path),
		Line:     line,
		EndLine:  end,
		Severity: sev,
		Rule:     rule,
		Title:    strings.TrimSpace(oneLine(title, 240)),
		Body:     strings.TrimSpace(body),
	}
	if key := firstString(m, "key", "id", "commentId"); key != "" && !isLowEntropyID(key) {
		f.Key = key
	} else {
		f.Key = findingKey(f.Path, f.Line, f.Rule, f.Title)
	}
	return f, true
}

func isLowEntropyID(s string) bool {
	// 纯数字序号不稳定，不用作跨次审查的 key
	_, err := strconv.Atoi(s)
	return err == nil
}

func findingKey(path string, line int, rule, title string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%d\x00%s\x00%s", path, line, rule, title)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func normalizeSeverity(s string) string {
	switch s {
	case "critical", "fatal", "blocker", "p0":
		return "critical"
	case "high", "error", "major", "p1":
		return "high"
	case "medium", "warn", "warning", "moderate", "p2":
		return "medium"
	case "low", "info", "note", "minor", "p3":
		return "low"
	case "":
		return "medium"
	default:
		return s
	}
}

func firstString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return t
				}
			case json.Number:
				return t.String()
			case float64:
				return strconv.FormatInt(int64(t), 10)
			}
		}
	}
	return ""
}

func firstInt(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return int(t)
			case json.Number:
				n, _ := t.Int64()
				return int(n)
			case int:
				return t
			case string:
				n, _ := strconv.Atoi(strings.TrimSpace(t))
				return n
			}
		}
	}
	return 0
}

func oneLine(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	return s[:n]
}
