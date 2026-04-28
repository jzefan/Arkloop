package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkoukk/tiktoken-go"
)

func NewRuntimeContextMiddleware() RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		if rc.ChannelContext != nil {
			rc.UpsertPromptSegment(PromptSegment{
				Name:          "runtime.channel_output_behavior",
				Target:        PromptTargetSystemPrefix,
				Role:          "system",
				Text:          buildChannelOutputBehaviorBlock(),
				Stability:     PromptStabilityStablePrefix,
				CacheEligible: true,
			})
			isAdmin := checkSenderIsAdmin(ctx, rc)
			rc.SenderIsAdmin = isAdmin
		}
		if rc.ResumePromptSnapshot != nil {
			rc.ApplyResumePromptSnapshot()
			return next(ctx, rc)
		}
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.context",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildRuntimeContextBlock(ctx, rc),
			Stability:     PromptStabilitySessionPrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.tool_usage_guidance",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildToolUsageGuidanceBlock(),
			Stability:     PromptStabilitySessionPrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.doing_tasks",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildDoingTasksBlock(),
			Stability:     PromptStabilityStablePrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.tone_and_style",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildToneAndStyleBlock(),
			Stability:     PromptStabilityStablePrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.executing_actions",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildExecutingActionsBlock(),
			Stability:     PromptStabilityStablePrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.git_workflow",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildGitWorkflowBlock(),
			Stability:     PromptStabilityStablePrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.conversation_mechanics",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildConversationMechanicsBlock(),
			Stability:     PromptStabilityStablePrefix,
			CacheEligible: true,
		})
		rc.UpsertPromptSegment(PromptSegment{
			Name:          "runtime.first_turn",
			Target:        PromptTargetSystemPrefix,
			Role:          "system",
			Text:          buildFirstTurnGuidanceBlock(),
			Stability:     PromptStabilityStablePrefix,
			CacheEligible: true,
		})
		return next(ctx, rc)
	}
}

func buildChannelOutputBehaviorBlock() string {
	return `<channel_output_behavior>
Your text outputs are delivered to the chat platform in real-time as separate messages.
When you call tools mid-reply, text before and after the tool call becomes distinct messages visible to the user.
Avoid repeating content that was already sent. If you have nothing new to add after a tool call, use end_reply.
</channel_output_behavior>`
}

func buildToolUsageGuidanceBlock() string {
	return `<tool_usage_guidance>
Prefer dedicated tools over exec_command for all file and search operations. Dedicated tools are safer, respect sandbox boundaries, and produce structured results that the system can process correctly.

Tool substitution rules:
- Use Read instead of exec_command with cat, head, tail, or less — Read supports offset/limit for large files
- Use Edit or Write instead of exec_command with sed, awk, echo >, heredocs, or shell redirects
- Use Glob instead of exec_command with find, fd, ls, or dir
- Use Grep instead of exec_command with grep, rg, or ag
- Use WebFetch to retrieve URLs instead of exec_command with curl or wget

Forbidden behaviors:
- Do NOT use shell redirection (>, >>, | tee) to write files — use Write or Edit
- Do NOT redirect command output to temporary files (e.g., "git diff > /tmp/out.txt") to work around output length limits. If a tool result is large, the system persists it automatically and provides a filepath you can Read.
- Do NOT repeatedly Read a persisted output file to extract fragments — use Grep to search it, or Read with offset/limit to page through it.
- Do NOT use cat, head, tail, sed, or awk for file operations that Read, Edit, or Write can handle directly.

Parallel tool calls:
You may call multiple tools in a single response when they are independent of each other. If one call depends on the output of another, call them sequentially instead.

Output persistence:
When a tool produces large output, the system may persist the full content to disk and replace the inline result with a preview. The result will contain "persisted": true, "filepath", "original_bytes", and "preview" fields. Use the filepath with Read (with offset/limit) or Grep to work with the persisted content efficiently.

Output management strategy:
When a tool result is large or persisted, do NOT attempt to read the entire file at once. Instead:
1. Use Grep to search for specific content (error messages, function names, keywords)
2. Use Read with offset/limit to page through sections
3. Only read the full file if you need every line and it fits within your context budget
For command output: the preview shows the first and last portions — the middle is truncated. The tail often contains the most relevant output (results, summaries, error details).

Reserve exec_command for operations that genuinely require a shell: running build systems, package managers, compilers, git commands, linters, or long-running processes.
</tool_usage_guidance>`
}

func buildDoingTasksBlock() string {
	return `<doing_tasks>
You will primarily perform software engineering tasks: solving bugs, adding features, refactoring code, explaining code, and more. When given an unclear or generic instruction, consider it in the context of these software engineering tasks and the current working directory.

You are highly capable and often allow users to complete ambitious tasks that would otherwise be too complex or take too long. Defer to user judgment about whether a task is too large to attempt.

Code style:
- Don't add features, refactor, or make "improvements" beyond what was asked. A bug fix doesn't need surrounding code cleaned up. A simple feature doesn't need extra configurability.
- Don't add error handling, fallbacks, or validation for scenarios that can't happen. Trust internal code and framework guarantees. Only validate at system boundaries (user input, external APIs).
- Don't create helpers, utilities, or abstractions for one-time operations. Don't design for hypothetical future requirements. Three similar lines is better than a premature abstraction.
- Default to writing no comments. Only add one when the WHY is non-obvious: a hidden constraint, a subtle invariant, a workaround for a specific bug, behavior that would surprise a reader.
- Avoid backwards-compatibility hacks like renaming unused _vars, re-exporting types, adding // removed comments. If something is unused, delete it completely.
- Before reporting a task complete, verify it actually works. If you can't verify (no test exists, can't run the code), say so explicitly rather than claiming success.

If an approach fails, diagnose why before switching tactics — read the error, check your assumptions, try a focused fix. Don't blindly retry the identical action, but don't abandon a viable approach after a single failure either.

Be careful not to introduce security vulnerabilities such as command injection, XSS, SQL injection, and other OWASP top 10 vulnerabilities. If you notice that you wrote insecure code, immediately fix it. Prioritize writing safe, secure, and correct code.

Do not create files unless absolutely necessary. Prefer editing existing files to creating new ones, as this prevents file bloat.

Avoid giving time estimates or predictions for how long tasks will take. Focus on what needs to be done, not how long it might take.
</doing_tasks>`
}

func buildToneAndStyleBlock() string {
	return `<tone_and_style>
- Do not use emojis unless the user explicitly requests it. Avoid using emojis in all communication unless asked.
- Keep responses short and concise.
- When referencing specific functions or pieces of code include the pattern file_path:line_number to allow the user to easily navigate to the source code location.
- Do not use a colon before tool calls. Your tool calls may not be shown directly in the output, so text like "Let me read the file:" followed by a read tool call should just be "Let me read the file." with a period.
</tone_and_style>`
}

func buildExecutingActionsBlock() string {
	return `<executing_actions>
Carefully consider the reversibility and blast radius of actions before executing them. Generally you can freely take local, reversible actions like editing files or running tests. But for actions that are hard to reverse, affect shared systems beyond your local environment, or could otherwise be risky or destructive, check with the user before proceeding.

Risky actions that warrant user confirmation:
- Destructive operations: deleting files/branches, dropping database tables, killing processes, rm -rf, overwriting uncommitted changes
- Hard-to-reverse operations: force-pushing, git reset --hard, amending published commits, removing or downgrading packages, modifying CI/CD pipelines
- Actions visible to others: pushing code, creating/closing/commenting on PRs or issues, sending messages, modifying shared infrastructure or permissions
- Uploading content to third-party web tools publishes it — consider whether it could be sensitive before sending, since it may be cached or indexed even if later deleted

A user approving an action (like a git push) once does NOT mean that they approve it in all contexts. Authorization stands for the scope specified, not beyond. Match the scope of your actions to what was actually requested.

When you encounter an obstacle, do not use destructive actions as a shortcut to simply make it go away. Identify root causes and fix underlying issues rather than bypassing safety checks. If you discover unexpected state like unfamiliar files, branches, or configuration, investigate before deleting or overwriting — it may represent the user's in-progress work.
</executing_actions>`
}

func buildGitWorkflowBlock() string {
	return `<git_workflow>
Only create commits when requested by the user. If unclear, ask first.

When the user asks you to create a git commit:
1. Run the following commands in parallel to understand the current state:
   - git status (never use -uall flag — it causes memory issues on large repos)
   - git diff (to see both staged and unstaged changes)
   - git log --oneline -5 (to follow the repo's commit message style)
2. Analyze all staged changes and draft a commit message:
   - Summarize the nature of the changes (feat, fix, refactor, docs, test, chore, perf, ci)
   - Do not commit files that contain secrets (.env, credentials.json, etc). Warn if asked.
   - Draft a concise (1-2 sentences) message focused on "why" rather than "what"
3. Run the following:
   - Add relevant untracked files by name (NOT "git add -A" or "git add .")
   - Create the commit with the message. Use a HEREDOC for formatting:
     git commit -m "$(cat <<'EOF'
     type: description
     EOF
     )"
   - Run git status to verify success
4. If the commit fails due to a pre-commit hook: fix the issue and create a NEW commit (do NOT use --no-verify)

Git Safety Protocol:
- NEVER skip hooks (--no-verify, --no-gpg-sign) unless the user explicitly asks
- NEVER force push to main/master — warn the user if they request it
- Always create NEW commits, never amend unless the user explicitly requests it. If a pre-commit hook fails, the commit did NOT happen, so --amend would modify the PREVIOUS commit and destroy work.
- NEVER run destructive commands (push --force, reset --hard, checkout -- ., clean -f, branch -D) unless the user explicitly requests them
- Do NOT push to the remote repository unless the user explicitly asks

Creating pull requests:
1. First run in parallel: git status, git diff, check remote tracking, git log + git diff [base]...HEAD
2. Analyze ALL commits (not just the latest one) and draft a PR:
   - Title under 70 characters
   - Body with ## Summary and ## Test plan sections
3. Create branch if needed, push with -u flag, then:
   gh pr create --title "title" --body "$(cat <<'EOF'
   ## Summary
   ...

   ## Test plan
   ...
   EOF
   )"

If there are no changes to commit, do not create an empty commit.
</git_workflow>`
}

func buildConversationMechanicsBlock() string {
	return `<conversation_mechanics>
The system will automatically compress prior messages in this conversation as it approaches context limits. When compression occurs, older messages are replaced with a summary — you may notice that earlier details are no longer present. This is normal and helps keep the conversation within token limits.

Tool results and user messages may include <system-reminder> or other system tags. These tags are added by the system automatically and are not part of the user's input or the tool's output. They convey system-level information such as plan mode status, context limits, or memory availability.

Each tool call and its result count as one or more conversation turns. Minimize the total number of turns by batching independent operations into a single response. Fewer turns means lower cumulative token cost, since every turn carries the full conversation history.

When a tool result includes "persisted": true, the full output has been saved to disk and only a preview is shown inline. Use the filepath field with read (with offset/limit) or grep to work with the persisted content efficiently. Do not re-read the entire persisted file unless you need all of it.
</conversation_mechanics>`
}

func buildFirstTurnGuidanceBlock() string {
	return `<first_turn_guidance>
On your very first response in a new conversation:
1. Read the user's message carefully and understand their intent before taking action.
2. If the task is clear and specific, proceed directly — do not ask unnecessary clarifying questions.
3. If the task is ambiguous or underspecified, state your interpretation and proceed — only ask when the task has genuinely conflicting interpretations.
4. Do not explore codebase areas unrelated to the task — focus on what the user asked for.
5. When you do start working, prefer reading files before editing them. Understand existing code before modifying it.
6. Batch your initial exploration: if you need to understand multiple files, read them in parallel rather than one at a time.
</first_turn_guidance>`
}

func buildRuntimeContextBlock(ctx context.Context, rc *RunContext) string {
	if rc == nil {
		return ""
	}

	timeZone := runtimeContextTimeZone(ctx, rc)
	loc := loadRuntimeLocation(timeZone)
	localDate := time.Now().UTC().In(loc).Format("2006-01-02")

	var sb strings.Builder
	sb.WriteString("<runtime_context>\n")
	sb.WriteString("User Timezone: " + timeZone + "\n")
	sb.WriteString("User Local Date: " + localDate + "\n")
	sb.WriteString("Host Mode: " + hostMode + "\n")
	sb.WriteString("Platform: " + runtime.GOOS + "/" + runtime.GOARCH)
	if hostMode == "desktop" {
		sb.WriteString("\nExecution Environment: local machine (commands run directly on the user's device, not in a cloud sandbox)")
	}

	if rc.AgentConfig != nil && rc.AgentConfig.Model != nil && strings.TrimSpace(*rc.AgentConfig.Model) != "" {
		sb.WriteString("\nModel: " + strings.TrimSpace(*rc.AgentConfig.Model))
	}

	if rc.WorkDir != "" {
		sb.WriteString("\nWorking Directory: " + rc.WorkDir)

		if shell := os.Getenv("SHELL"); shell != "" {
			sb.WriteString("\nShell: " + shell)
		}

		isRepo := runtimeGitIsRepo(rc.WorkDir)
		sb.WriteString("\nGit Repository: " + fmt.Sprintf("%t", isRepo))

		if isRepo {
			sb.WriteString("\n<git>")
			sb.WriteString(runtimeGitContext(rc.WorkDir))
			sb.WriteString("\n</git>")
		}

		if tree := runtimeDirTree(rc.WorkDir); tree != "" {
			sb.WriteString("\n\n<directory_tree>\n" + tree + "</directory_tree>")
		}

		if mem := runtimeProjectInstructions(rc.WorkDir, isRepo); mem != "" {
			sb.WriteString("\n\n" + mem)
		}
	}

	sb.WriteString("\n</runtime_context>")
	return sb.String()
}

func formatBotIdentity(cc *ChannelContext) string {
	name := cc.BotDisplayName
	uname := cc.BotUsername
	if name == "" && uname == "" {
		return ""
	}
	if name != "" && uname != "" {
		return fmt.Sprintf("%s (@%s)", name, uname)
	}
	if uname != "" {
		return "@" + uname
	}
	return name
}

func formatRuntimeLocalNow(now time.Time, timeZone string) string {
	loc := loadRuntimeLocation(timeZone)
	local := now.In(loc)
	return local.Format("2006-01-02 15:04:05") + " [" + formatRuntimeUTCOffset(local) + "]"
}

func loadRuntimeLocation(timeZone string) *time.Location {
	cleaned := strings.TrimSpace(timeZone)
	if cleaned == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(cleaned)
	if err != nil {
		return time.UTC
	}
	return loc
}

// git helpers

func runtimeGitIsRepo(workDir string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = workDir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func runtimeGitContext(workDir string) string {
	run := func(args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workDir
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	branch := run("rev-parse", "--abbrev-ref", "HEAD")
	defaultBranch := runtimeGitDefaultBranch(workDir)
	username := run("config", "user.name")
	status := run("status", "--short")
	recentLog := run("log", "--oneline", "-5")

	if len(status) > 2000 {
		status = status[:2000] + "\n... (truncated)"
	}

	var sb strings.Builder
	if branch != "" {
		sb.WriteString("\nCurrent Branch: " + branch)
	}
	if defaultBranch != "" {
		sb.WriteString("\nDefault Branch: " + defaultBranch)
	}
	if username != "" {
		sb.WriteString("\nGit User: " + username)
	}
	if recentLog != "" {
		sb.WriteString("\nRecent Commits:\n" + recentLog)
	}
	if status != "" {
		sb.WriteString("\nGit Status:\n" + status)
	}
	return sb.String()
}

func runtimeGitDefaultBranch(workDir string) string {
	run := func(args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = workDir
		out, err := cmd.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}

	// try origin HEAD
	ref := run("symbolic-ref", "refs/remotes/origin/HEAD")
	if ref != "" {
		parts := strings.Split(ref, "/")
		return parts[len(parts)-1]
	}
	// fallback: check main, then master
	if run("rev-parse", "--verify", "refs/heads/main") != "" {
		return "main"
	}
	if run("rev-parse", "--verify", "refs/heads/master") != "" {
		return "master"
	}
	return ""
}

// find git root for AGENTS.md walk
func runtimeGitRoot(workDir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// tiktoken for token estimation

var (
	runtimeTiktokenOnce sync.Once
	runtimeTiktokenEnc  *tiktoken.Tiktoken
)

func runtimeEstimateTokens(text string) int {
	runtimeTiktokenOnce.Do(func() {
		enc, err := tiktoken.GetEncoding(tiktoken.MODEL_CL100K_BASE)
		if err == nil {
			runtimeTiktokenEnc = enc
		}
	})
	if runtimeTiktokenEnc == nil {
		return len(text) / 4
	}
	return len(runtimeTiktokenEnc.Encode(text, nil, nil))
}

// directory tree

const (
	dirTreeMaxDepth  = 2
	dirTreeMaxPerDir = 20
	dirTreeMaxTokens = 1600
	dirTreeMaxChars  = dirTreeMaxTokens * 4 // char proxy for fast per-line check
)

var dirTreeIgnore = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true,
	".venv": true, "venv": true, "vendor": true, "dist": true,
	"build": true, ".DS_Store": true, ".next": true, ".nuxt": true,
	".cache": true, "coverage": true, ".idea": true, ".vscode": true,
	"target": true, ".gradle": true,
}

func runtimeDirTree(root string) string {
	var sb strings.Builder
	sb.WriteString("Directory Structure:\n")
	charCount := sb.Len()
	truncated := runtimeDirTreeRecurse(&sb, &charCount, root, "", 0)

	result := sb.String()
	// final token-based check
	if runtimeEstimateTokens(result) > dirTreeMaxTokens {
		// trim to char proxy and mark truncated
		if len(result) > dirTreeMaxChars {
			result = result[:dirTreeMaxChars]
		}
		truncated = true
	}
	if truncated {
		result += "... (truncated)\n"
	}
	return result
}

func runtimeDirTreeRecurse(sb *strings.Builder, charCount *int, dir, prefix string, depth int) bool {
	if depth >= dirTreeMaxDepth {
		return false
	}

	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	entries, err := f.Readdir(-1)
	_ = f.Close()
	if err != nil {
		return false
	}

	// filter and sort
	var filtered []os.FileInfo
	for _, e := range entries {
		if dirTreeIgnore[e.Name()] {
			continue
		}
		filtered = append(filtered, e)
	}
	sort.Slice(filtered, func(i, j int) bool {
		// dirs first, then alphabetical
		di, dj := filtered[i].IsDir(), filtered[j].IsDir()
		if di != dj {
			return di
		}
		return filtered[i].Name() < filtered[j].Name()
	})

	total := len(filtered)
	show := total
	if show > dirTreeMaxPerDir {
		show = dirTreeMaxPerDir - 1 // show 19 + summary
	}

	for i := 0; i < show; i++ {
		e := filtered[i]
		isLast := i == total-1 || (total > dirTreeMaxPerDir && i == show-1)
		connector := "├── "
		childPrefix := prefix + "│   "
		if isLast && total <= dirTreeMaxPerDir {
			connector = "└── "
			childPrefix = prefix + "    "
		}

		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		line := prefix + connector + name + "\n"
		*charCount += len(line)
		if *charCount > dirTreeMaxChars {
			return true
		}
		sb.WriteString(line)

		if e.IsDir() {
			if runtimeDirTreeRecurse(sb, charCount, filepath.Join(dir, e.Name()), childPrefix, depth+1) {
				return true
			}
		}
	}

	if total > dirTreeMaxPerDir {
		remaining := total - show
		line := prefix + "└── ... (" + fmt.Sprintf("%d", remaining) + " more)\n"
		*charCount += len(line)
		if *charCount > dirTreeMaxChars {
			return true
		}
		sb.WriteString(line)
	}

	return false
}

// AGENTS.md project instructions

func runtimeProjectInstructions(workDir string, isRepo bool) string {
	var stopAt string
	if isRepo {
		stopAt = runtimeGitRoot(workDir)
	}

	// walk up from workDir, collect AGENTS.md paths
	var paths []string
	cur := filepath.Clean(workDir)
	for {
		candidate := filepath.Join(cur, "AGENTS.md")
		if _, err := os.Stat(candidate); err == nil {
			paths = append(paths, candidate)
		}
		if stopAt != "" && cur == filepath.Clean(stopAt) {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	if len(paths) == 0 {
		return ""
	}

	// token-based budgets
	const maxTotalTokens = 2000
	const maxPerFileTokens = 4096
	var sb strings.Builder
	totalTokens := 0

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := string(data)
		if runtimeEstimateTokens(content) > maxPerFileTokens {
			// truncate by chars as rough proxy, then re-check
			charLimit := maxPerFileTokens * 4
			if len(content) > charLimit {
				content = content[:charLimit]
			}
		}

		block := "<project_instructions>\n(contents of AGENTS.md from " + filepath.Dir(p) + ")\n" + content + "\n</project_instructions>"
		blockTokens := runtimeEstimateTokens(block)

		if totalTokens+blockTokens > maxTotalTokens {
			remaining := maxTotalTokens - totalTokens
			if remaining > 0 {
				// rough char-level truncation for remaining budget
				charBudget := remaining * 4
				if charBudget > len(block) {
					charBudget = len(block)
				}
				sb.WriteString(block[:charBudget])
			}
			break
		}
		sb.WriteString(block)
		totalTokens += blockTokens
	}

	return sb.String()
}

func formatRuntimeUTCOffset(t time.Time) string {
	_, offsetSeconds := t.Zone()
	sign := "+"
	if offsetSeconds < 0 {
		sign = "-"
		offsetSeconds = -offsetSeconds
	}
	hours := offsetSeconds / 3600
	minutes := (offsetSeconds % 3600) / 60
	if minutes == 0 {
		return fmt.Sprintf("UTC%s%d", sign, hours)
	}
	return fmt.Sprintf("UTC%s%d:%02d", sign, hours, minutes)
}
