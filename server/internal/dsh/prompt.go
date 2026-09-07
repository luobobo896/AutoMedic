package dsh

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/automedic/automedic/internal/model"
)

// BuildContext 修复上下文输入
type BuildContext struct {
	Project     *model.Project
	Repo        *model.Repository
	Event       *model.Event
	Rule        *model.Rule
	Workspace   string
	TaskID      uint
	MaxTreeLine int
	Extra       string
}

// BuildTaskText 生成交给 dsh headless 的自包含任务文本
// headless 只接受一个 task 参数，因此所有上下文必须内联。
func BuildTaskText(ctx BuildContext) string {
	var b strings.Builder
	b.WriteString("# 任务：修复生产环境缺陷\n\n")
	b.WriteString("你是 AutoMedic 的自动修复引擎，运行在隔离的 git 工作区中。请定位下面这个生产缺陷的根因，并**直接完成最小必要的代码修复**。\n\n")

	b.WriteString("## 一、事件信息\n")
	if ctx.Event != nil {
		fmt.Fprintf(&b, "- 事件 ID：#%d\n", ctx.Event.ID)
		fmt.Fprintf(&b, "- 标题：%s\n", oneLine(ctx.Event.Title))
		fmt.Fprintf(&b, "- 级别：%s\n", ctx.Event.Level)
		fmt.Fprintf(&b, "- 来源：%s\n", ctx.Event.Source)
		fmt.Fprintf(&b, "- 发生时间：%s\n", ctx.Event.OccurredAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&b, "- 指纹：%s\n", ctx.Event.Fingerprint)
		if ctx.Event.Message != "" {
			b.WriteString("\n### 事件详情\n\n")
			b.WriteString(truncateText(ctx.Event.Message, 8000))
			b.WriteString("\n")
		}
		if ctx.Event.Stack != "" {
			b.WriteString("\n### 堆栈 / 日志上下文\n\n```\n")
			b.WriteString(truncateText(ctx.Event.Stack, 12000))
			b.WriteString("\n```\n")
		}
	}

	if ctx.Project != nil {
		b.WriteString("\n## 二、项目背景\n")
		fmt.Fprintf(&b, "- 项目：%s（%s）\n", ctx.Project.Name, ctx.Project.Key)
		if ctx.Project.Context != "" {
			b.WriteString("- 业务上下文：\n")
			b.WriteString(indent(ctx.Project.Context))
			b.WriteString("\n")
		}
	}
	if ctx.Repo != nil {
		b.WriteString("\n## 三、仓库\n")
		fmt.Fprintf(&b, "- 仓库：%s\n", ctx.Repo.Name)
		fmt.Fprintf(&b, "- 分支：%s\n", ctx.Repo.Branch)
		if ctx.Repo.Language != "" {
			fmt.Fprintf(&b, "- 主要语言：%s\n", ctx.Repo.Language)
		}
		if ctx.Repo.CodePaths != "" {
			fmt.Fprintf(&b, "- 重点关注路径：%s\n", ctx.Repo.CodePaths)
		}
		b.WriteString("- 工作目录：当前目录\n")
	}

	if tree := repoTree(ctx.Workspace, ctx.MaxTreeLine); tree != "" {
		b.WriteString("\n## 四、仓库结构（顶层，供定位）\n\n```\n")
		b.WriteString(tree)
		b.WriteString("```\n")
	}

	if ctx.Extra != "" {
		b.WriteString("\n## 五、附加约束\n")
		b.WriteString(ctx.Extra)
		b.WriteString("\n")
	}

	b.WriteString("\n## 六、修复要求（必须遵守）\n")
	b.WriteString(`1. 先定位根因，再动代码；不要猜测式修改，不要大面积重构。
2. 只修复影响主流程的内部缺陷与真实代码 bug，重点排查：空指针/空值解引用、未处理的 error、并发竞态、边界条件、资源未释放、错误的异常吞掉。
3. 属于业务拒绝、第三方服务故障、配置/数据问题、权限问题的，不要改代码，在结果里说明并判定为 no_code_change。
4. 修改范围最小化，禁止顺手改动：依赖版本、CI 配置、格式化、无关文件；禁止删除或改写既有测试来让检查通过。
5. 补齐必要的错误处理与边界判断，必要时补充单元测试；保持仓库既有代码风格与命名习惯。
6. 修改完成后必须验证：能编译/静态检查通过，或运行相关测试；无法验证时在结果中说明原因。
7. 不要执行 git commit / git push / git 写操作，提交与推送由平台完成。
`)
	fmt.Fprintf(&b, `
## 七、输出要求（必须）
修复结束后，把结果写入当前目录下的 `+"`.automedic/result.json`"+`，同时在最终回复中复述摘要：

`+"```json"+`
{
  "diagnosis": "根因分析，200 字以内",
  "summary": "修复摘要：改了什么、为什么这样改、如何验证",
  "changed_files": ["path/to/file.go"],
  "confidence": "high | medium | low",
  "verification": "执行的验证命令与结果",
  "no_code_change": false
}
`+"```"+`

最终回复末尾追加固定格式的摘要块：

===AUTOMEDIC_RESULT===
DIAGNOSIS: <根因，一行>
SUMMARY: <修复摘要，一行>
FILES: <以逗号分隔的变更文件相对路径>
===AUTOMEDIC_END===

若判定无需改代码（no_code_change=true），同样输出上述块，SUMMARY 说明原因与建议。
任务编号：#%d
`, ctx.TaskID)
	return b.String()
}

// repoTree 生成仓库顶层结构（限深度 2，跳过 .git）
func repoTree(root string, maxLines int) string {
	if root == "" {
		return ""
	}
	if maxLines <= 0 {
		maxLines = 120
	}
	var sb strings.Builder
	n := 0
	var walk func(dir, prefix string, depth int)
	walk = func(dir, prefix string, depth int) {
		if n >= maxLines || depth > 2 {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			if n >= maxLines {
				return
			}
			name := e.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".automedic" {
				continue
			}
			sb.WriteString(prefix + name)
			if e.IsDir() {
				sb.WriteString("/\n")
				n++
				walk(filepath.Join(dir, name), prefix+"  ", depth+1)
			} else {
				sb.WriteString("\n")
				n++
			}
		}
	}
	walk(root, "", 0)
	return sb.String()
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", ""))
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}
