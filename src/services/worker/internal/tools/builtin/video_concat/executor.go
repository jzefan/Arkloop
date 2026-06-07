// Package videoconcat concatenates multiple mp4 artifacts into one final mp4
// using the local ffmpeg binary (installed via the worker Dockerfile).
//
// Strategy:
//
//  1. Resolve each input artifact reference, sanity-check account ownership,
//     download bytes from the object store, write to a private temp dir.
//  2. Build an ffmpeg "concat demuxer" list file referencing each temp file.
//  3. Run ffmpeg either in stream-copy mode (default; fast, requires identical
//     codecs/resolutions — which Seedance always produces in one batch) or
//     with H.264 re-encode (when caller passes reencode=true).
//  4. Read the resulting mp4 and persist it as a run-scoped artifact.
package videoconcat

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

const (
	defaultArtifactName = "final_video"
	ffmpegTimeout       = 180 * time.Second
)

type ToolExecutor struct {
	store objectstore.Store
}

func NewToolExecutor(store objectstore.Store) *ToolExecutor {
	return &ToolExecutor{store: store}
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()
	if e == nil || e.store == nil {
		return errResult("tool.not_configured", "video concat storage is not configured", started)
	}
	if execCtx.AccountID == nil {
		return errResult("tool.execution_failed", "account context is required", started)
	}
	rawInputs, _ := args["inputs"].([]any)
	if len(rawInputs) < 1 {
		return errResult("tool.args_invalid", "inputs must be an array of at least 1 artifact reference", started)
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errResult("tool.not_configured", "ffmpeg binary not found in worker image", started)
	}

	tmpDir, err := os.MkdirTemp("", "video_concat-")
	if err != nil {
		return errResult("tool.execution_failed", "create temp dir: "+err.Error(), started)
	}
	defer os.RemoveAll(tmpDir)

	// 1. 下载每个 input artifact 到 tmpDir
	localPaths := make([]string, 0, len(rawInputs))
	for idx, raw := range rawInputs {
		ref, ok := raw.(string)
		if !ok {
			return errResult("tool.args_invalid", fmt.Sprintf("inputs[%d] must be a string", idx), started)
		}
		key := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), "artifact:"))
		if key == "" {
			return errResult("tool.args_invalid", fmt.Sprintf("inputs[%d] is empty", idx), started)
		}
		if !strings.HasPrefix(key, execCtx.AccountID.String()+"/") {
			return errResult("tool.args_invalid", fmt.Sprintf("inputs[%d] is outside the current account", idx), started)
		}
		data, _, err := e.store.GetWithContentType(ctx, key)
		if err != nil {
			return errResult("tool.execution_failed", fmt.Sprintf("inputs[%d] download failed: %s", idx, err.Error()), started)
		}
		localPath := filepath.Join(tmpDir, fmt.Sprintf("part_%03d.mp4", idx))
		if err := os.WriteFile(localPath, data, 0o600); err != nil {
			return errResult("tool.execution_failed", fmt.Sprintf("write inputs[%d] to tmp: %s", idx, err.Error()), started)
		}
		localPaths = append(localPaths, localPath)
	}

	// 3. 跑 ffmpeg
	outPath := filepath.Join(tmpDir, "out.mp4")
	audioRef := strings.TrimSpace(stringArg(args, "audio"))
	audioPath := ""
	if audioRef != "" {
		key := strings.TrimSpace(strings.TrimPrefix(audioRef, "artifact:"))
		if key == "" {
			return errResult("tool.args_invalid", "audio is empty", started)
		}
		if !strings.HasPrefix(key, execCtx.AccountID.String()+"/") {
			return errResult("tool.args_invalid", "audio is outside the current account", started)
		}
		data, contentType, err := e.store.GetWithContentType(ctx, key)
		if err != nil {
			return errResult("tool.execution_failed", "audio download failed: "+err.Error(), started)
		}
		audioPath = filepath.Join(tmpDir, "narration_audio"+audioFileExt(contentType))
		if err := os.WriteFile(audioPath, data, 0o600); err != nil {
			return errResult("tool.execution_failed", "write audio to tmp: "+err.Error(), started)
		}
	}
	reencode := false
	if v, ok := args["reencode"].(bool); ok {
		reencode = v
	}
	transition := strings.ToLower(strings.TrimSpace(stringArg(args, "transition")))
	if transition == "" {
		transition = "none"
	}
	if transition != "none" && transition != "crossfade" {
		return errResult("tool.args_invalid", "transition must be none or crossfade", started)
	}
	transitionSeconds := numberArg(args, "transition_seconds", 0.35)
	if transitionSeconds < 0.1 {
		transitionSeconds = 0.1
	}
	if transitionSeconds > 1.0 {
		transitionSeconds = 1.0
	}

	ffCtx, cancel := context.WithTimeout(ctx, ffmpegTimeout)
	defer cancel()
	var ffArgs []string
	if transition == "crossfade" {
		durations := make([]float64, 0, len(localPaths))
		for idx, p := range localPaths {
			duration, err := probeDurationSeconds(ffCtx, p)
			if err != nil {
				return errResult("tool.execution_failed", fmt.Sprintf("probe duration for inputs[%d]: %s", idx, err.Error()), started)
			}
			if duration <= transitionSeconds+0.1 {
				return errResult("tool.args_invalid", fmt.Sprintf("inputs[%d] duration is too short for crossfade", idx), started)
			}
			durations = append(durations, duration)
		}
		ffArgs = buildCrossfadeArgs(localPaths, durations, transitionSeconds, outPath)
	} else {
		// 2. 构造 concat demuxer 列表文件
		listFile := filepath.Join(tmpDir, "concat.txt")
		var listBuf strings.Builder
		for _, p := range localPaths {
			// ffmpeg concat demuxer 要求 'file <path>'，单引号内禁止单引号
			listBuf.WriteString(fmt.Sprintf("file '%s'\n", p))
		}
		if err := os.WriteFile(listFile, []byte(listBuf.String()), 0o600); err != nil {
			return errResult("tool.execution_failed", "write concat list: "+err.Error(), started)
		}

		if reencode {
			// 重编码模式：兼容混合分辨率/编码
			ffArgs = []string{
				"-y", "-hide_banner", "-loglevel", "error",
				"-f", "concat", "-safe", "0", "-i", listFile,
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "22",
				"-c:a", "aac", "-b:a", "128k",
				"-movflags", "+faststart",
				outPath,
			}
		} else {
			// 流复制模式：默认。Seedance 同批输出的 mp4 编码一致，可以直接 stream-copy。
			ffArgs = []string{
				"-y", "-hide_banner", "-loglevel", "error",
				"-f", "concat", "-safe", "0", "-i", listFile,
				"-c", "copy",
				"-movflags", "+faststart",
				outPath,
			}
		}
	}
	cmd := exec.CommandContext(ffCtx, "ffmpeg", ffArgs...)
	stderr, runErr := cmd.CombinedOutput()
	if runErr != nil {
		// stream-copy 失败时自动 fallback 到重编码（编码/分辨率不一致的典型情况）
		if transition == "none" && !reencode {
			listFile := filepath.Join(tmpDir, "concat.txt")
			ffArgs = []string{
				"-y", "-hide_banner", "-loglevel", "error",
				"-f", "concat", "-safe", "0", "-i", listFile,
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "22",
				"-c:a", "aac", "-b:a", "128k",
				"-movflags", "+faststart",
				outPath,
			}
			ffCtx2, cancel2 := context.WithTimeout(ctx, ffmpegTimeout)
			defer cancel2()
			cmd2 := exec.CommandContext(ffCtx2, "ffmpeg", ffArgs...)
			stderr2, runErr2 := cmd2.CombinedOutput()
			if runErr2 != nil {
				return errResult("tool.execution_failed",
					fmt.Sprintf("ffmpeg concat failed in both stream-copy and reencode mode: copy_err=%s reencode_err=%s reencode_stderr=%s",
						runErr.Error(), runErr2.Error(), truncate(string(stderr2), 600)),
					started)
			}
		} else {
			return errResult("tool.execution_failed",
				fmt.Sprintf("ffmpeg concat failed: %s stderr=%s", runErr.Error(), truncate(string(stderr), 600)),
				started)
		}
	}

	audioAttached := false
	if audioPath != "" {
		audioMode := strings.ToLower(strings.TrimSpace(stringArg(args, "audio_mode")))
		if audioMode == "" {
			audioMode = "mix"
		}
		if audioMode != "mix" && audioMode != "replace" {
			return errResult("tool.args_invalid", "audio_mode must be mix or replace", started)
		}
		narrationVolume := clampNumber(numberArg(args, "narration_volume", 1.0), 0, 4)
		backgroundVolume := clampNumber(numberArg(args, "background_volume", 0.25), 0, 4)
		duration, err := probeDurationSeconds(ctx, outPath)
		if err != nil {
			return errResult("tool.execution_failed", "probe concat output duration: "+err.Error(), started)
		}
		hasVideoAudio, err := probeHasAudio(ctx, outPath)
		if err != nil {
			return errResult("tool.execution_failed", "probe concat output audio stream: "+err.Error(), started)
		}
		mixedPath := filepath.Join(tmpDir, "out_with_audio.mp4")
		mixExisting := audioMode == "mix" && hasVideoAudio && backgroundVolume > 0
		ffCtxAudio, cancelAudio := context.WithTimeout(ctx, ffmpegTimeout)
		defer cancelAudio()
		audioArgs := buildAttachAudioArgs(outPath, audioPath, mixedPath, duration, mixExisting, narrationVolume, backgroundVolume)
		cmdAudio := exec.CommandContext(ffCtxAudio, "ffmpeg", audioArgs...)
		stderrAudio, runAudioErr := cmdAudio.CombinedOutput()
		if runAudioErr != nil {
			return errResult("tool.execution_failed",
				fmt.Sprintf("ffmpeg attach audio failed: %s stderr=%s", runAudioErr.Error(), truncate(string(stderrAudio), 600)),
				started)
		}
		outPath = mixedPath
		audioAttached = true
	}

	// 4. 读取输出并存为 artifact
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		return errResult("tool.execution_failed", "read concat output: "+err.Error(), started)
	}
	if len(outBytes) == 0 {
		return errResult("tool.execution_failed", "concat output is empty", started)
	}

	artifactBase := sanitizeArtifactName(strings.TrimSpace(stringArg(args, "artifact_name")))
	if artifactBase == "" {
		artifactBase = defaultArtifactName
	}
	filename := artifactBase + ".mp4"
	key := buildArtifactKey(execCtx, filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		v := execCtx.ThreadID.String()
		threadID = &v
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), execCtx.AccountID.String(), threadID)
	if err := e.store.PutObject(ctx, key, outBytes, objectstore.PutOptions{
		ContentType: "video/mp4",
		Metadata:    metadata,
	}); err != nil {
		return errResult("tool.upload_failed", "save concat artifact: "+err.Error(), started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"mime_type":      "video/mp4",
			"bytes":          len(outBytes),
			"inputs":         len(localPaths),
			"transition":     transition,
			"audio_attached": audioAttached,
			"artifacts": []map[string]any{
				{
					"key":       key,
					"filename":  filename,
					"size":      len(outBytes),
					"mime_type": "video/mp4",
					"title":     artifactBase,
					"display":   "inline",
				},
			},
		},
		DurationMs: durationMs(started),
	}
}

// IsAvailableForAccount 仅在 ffmpeg 二进制可用时返回 true。
func (e *ToolExecutor) IsAvailableForAccount(_ context.Context, accountID uuid.UUID) bool {
	if accountID == uuid.Nil {
		return false
	}
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// ─── helpers ───

func stringArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func numberArg(args map[string]any, key string, fallback float64) float64 {
	switch v := args[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case jsonNumber:
		n, err := strconv.ParseFloat(string(v), 64)
		if err == nil {
			return n
		}
	}
	return fallback
}

type jsonNumber string

func clampNumber(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// durationRe matches the ffmpeg banner line "  Duration: 00:00:20.22, start: ...".
var durationRe = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)

// audioStreamRe matches "  Stream #0:1[0x2](und): Audio: aac ...".
var audioStreamRe = regexp.MustCompile(`Stream #\d+:\d+(?:\[[^\]]*\])?(?:\([^)]*\))?:\s*Audio:`)

// probeWithFfmpeg runs `ffmpeg -i <path>` and returns the combined banner output.
// ffmpeg exits non-zero when no output file is given — that's expected; the
// banner (which carries Duration + Stream lines) is still emitted. We therefore
// ignore the exit error and parse the captured text. This removes the hard
// dependency on a separate ffprobe binary, which the static ffmpeg build in the
// worker image does not ship.
func probeWithFfmpeg(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-i", path)
	out, _ := cmd.CombinedOutput() // exit 1 is normal (no output file); banner still printed
	text := string(out)
	if !strings.Contains(text, "Duration:") && !audioStreamRe.MatchString(text) {
		return "", fmt.Errorf("ffmpeg produced no probe info for %s: %s", path, truncate(text, 300))
	}
	return text, nil
}

func probeDurationSeconds(ctx context.Context, path string) (float64, error) {
	text, err := probeWithFfmpeg(ctx, path)
	if err != nil {
		return 0, err
	}
	m := durationRe.FindStringSubmatch(text)
	if m == nil {
		return 0, fmt.Errorf("ffmpeg banner has no Duration for %s", path)
	}
	h, _ := strconv.ParseFloat(m[1], 64)
	mn, _ := strconv.ParseFloat(m[2], 64)
	s, _ := strconv.ParseFloat(m[3], 64)
	return h*3600 + mn*60 + s, nil
}

func probeHasAudio(ctx context.Context, path string) (bool, error) {
	text, err := probeWithFfmpeg(ctx, path)
	if err != nil {
		return false, err
	}
	return audioStreamRe.MatchString(text), nil
}

func buildCrossfadeArgs(paths []string, durations []float64, transitionSeconds float64, outPath string) []string {
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	for _, p := range paths {
		args = append(args, "-i", p)
	}
	var filter strings.Builder
	for i := range paths {
		filter.WriteString(fmt.Sprintf("[%d:v]fps=30,format=yuv420p,settb=AVTB[v%d];", i, i))
	}
	compositeLabel := "v0"
	compositeDuration := durations[0]
	for i := 1; i < len(paths); i++ {
		outLabel := fmt.Sprintf("vx%d", i)
		offset := compositeDuration - transitionSeconds
		filter.WriteString(fmt.Sprintf("[%s][v%d]xfade=transition=fade:duration=%.3f:offset=%.3f[%s];",
			compositeLabel, i, transitionSeconds, offset, outLabel))
		compositeLabel = outLabel
		compositeDuration = compositeDuration + durations[i] - transitionSeconds
	}
	filterText := strings.TrimSuffix(filter.String(), ";")
	args = append(args,
		"-filter_complex", filterText,
		"-map", "["+compositeLabel+"]",
		"-an",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "22",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		outPath,
	)
	return args
}

func buildAttachAudioArgs(videoPath, audioPath, outPath string, videoDuration float64, mixExisting bool, narrationVolume, backgroundVolume float64) []string {
	args := []string{
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-i", audioPath,
	}
	duration := fmt.Sprintf("%.3f", videoDuration)
	if mixExisting {
		filter := fmt.Sprintf(
			"[0:a]volume=%.3f[background];[1:a]volume=%.3f,apad,atrim=0:%s,asetpts=N/SR/TB[narration];[background][narration]amix=inputs=2:duration=first:dropout_transition=0[audio]",
			backgroundVolume, narrationVolume, duration,
		)
		args = append(args,
			"-filter_complex", filter,
			"-map", "0:v:0",
			"-map", "[audio]",
			"-c:v", "copy",
			"-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart",
			outPath,
		)
		return args
	}
	filter := fmt.Sprintf("[1:a]volume=%.3f,apad,atrim=0:%s,asetpts=N/SR/TB[narration]", narrationVolume, duration)
	args = append(args,
		"-filter_complex", filter,
		"-map", "0:v:0",
		"-map", "[narration]",
		"-c:v", "copy",
		"-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart",
		outPath,
	)
	return args
}

func audioFileExt(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0])) {
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/ogg", "audio/opus":
		return ".opus"
	case "audio/aac":
		return ".aac"
	case "audio/flac":
		return ".flac"
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	default:
		return ".audio"
	}
}

func sanitizeArtifactName(raw string) string {
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '.':
			b.WriteByte('_')
		}
	}
	cleaned := strings.Trim(b.String(), "-_")
	if len(cleaned) > 80 {
		cleaned = cleaned[:80]
	}
	return cleaned
}

func buildArtifactKey(execCtx tools.ExecutionContext, filename string) string {
	accountID := "_anonymous"
	if execCtx.AccountID != nil {
		accountID = execCtx.AccountID.String()
	}
	return fmt.Sprintf("%s/%s/%s", accountID, execCtx.RunID.String(), filename)
}

func errResult(class, msg string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error:      &tools.ExecutionError{ErrorClass: class, Message: msg},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	d := time.Since(started)
	if d < 0 {
		return 0
	}
	return int(d / time.Millisecond)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
