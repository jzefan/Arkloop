package pipeline

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"arkloop/services/shared/messagecontent"
	"arkloop/services/worker/internal/llm"

	"github.com/google/uuid"
	"github.com/pkoukk/tiktoken-go"
)

type FrontierNodeKind string

const (
	FrontierNodeChunk       FrontierNodeKind = "chunk"
	FrontierNodeReplacement FrontierNodeKind = "replacement"
)

type FrontierNode struct {
	Kind            FrontierNodeKind
	NodeID          uuid.UUID
	Layer           int
	StartContextSeq int64
	EndContextSeq   int64
	StartThreadSeq  int64
	EndThreadSeq    int64
	SourceText      string
	ApproxTokens    int

	MsgStart  int
	MsgEnd    int
	AtomSeq   int
	AtomType  compactAtomType
	Role      string
	atomKey   string
	chunkSeq  int64
	chunkKind string
	role      string
}

type compactFrontierSelection struct {
	Nodes          []FrontierNode
	EndNodeIndex   int
	TargetTokens   int
	PartialTail    bool
	SelectedTokens int
}

func contextCompactTargetTokens(cfg ContextCompactSettings, window int) int {
	targetPct := cfg.TargetContextPct
	if targetPct <= 0 {
		targetPct = 75
	}
	if targetPct > 100 {
		targetPct = 100
	}
	if window <= 0 {
		window = cfg.FallbackContextWindowTokens
	}
	if window <= 0 {
		return 0
	}
	target := window * targetPct / 100
	if target < 1 {
		return 1
	}
	return target
}

func buildCompactFrontierNodesFromMessages(enc *tiktoken.Tiktoken, msgs []llm.Message) []FrontierNode {
	if len(msgs) == 0 {
		return nil
	}
	nodes := make([]FrontierNode, 0, len(msgs))
	nextContextSeq := int64(1)
	nextAtomSeq := 1
	for i := 0; i < len(msgs); {
		msg := msgs[i]
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			end := i + 1
			for end < len(msgs) && strings.TrimSpace(msgs[end].Role) == "tool" {
				end++
			}
			payload := serializeToolEpisodeForCompact(msgs[i:end])
			parts := splitCompactPayload(enc, payload)
			for _, part := range parts {
				nodes = append(nodes, FrontierNode{
					Kind:            FrontierNodeChunk,
					NodeID:          uuid.New(),
					StartContextSeq: nextContextSeq,
					EndContextSeq:   nextContextSeq,
					SourceText:      part,
					ApproxTokens:    compactTokenCount(enc, part),
					MsgStart:        i,
					MsgEnd:          end - 1,
					AtomSeq:         nextAtomSeq,
					AtomType:        compactAtomToolEpisode,
					Role:            "assistant",
				})
				nextContextSeq++
			}
			nextAtomSeq++
			i = end
			continue
		}
		if strings.TrimSpace(msg.Role) == "tool" {
			payload := serializeSingleToolForCompact(msg)
			parts := splitCompactPayload(enc, payload)
			for _, part := range parts {
				nodes = append(nodes, FrontierNode{
					Kind:            FrontierNodeChunk,
					NodeID:          uuid.New(),
					StartContextSeq: nextContextSeq,
					EndContextSeq:   nextContextSeq,
					SourceText:      part,
					ApproxTokens:    compactTokenCount(enc, part),
					MsgStart:        i,
					MsgEnd:          i,
					AtomSeq:         nextAtomSeq,
					AtomType:        compactAtomToolEpisode,
					Role:            "tool",
				})
				nextContextSeq++
			}
			nextAtomSeq++
			i++
			continue
		}

		rawText := strings.TrimSpace(messageText(msg))
		if rawText == "" {
			rawText = compactFallbackContentText(msg)
		}
		if rawText == "" {
			i++
			continue
		}
		atomType := compactAtomAssistantText
		if strings.TrimSpace(msg.Role) == "user" {
			atomType = compactAtomUserText
		}
		parts := splitCompactPayload(enc, rawText)
		for _, part := range parts {
			nodes = append(nodes, FrontierNode{
				Kind:            FrontierNodeChunk,
				NodeID:          uuid.New(),
				StartContextSeq: nextContextSeq,
				EndContextSeq:   nextContextSeq,
				SourceText:      part,
				ApproxTokens:    compactTokenCount(enc, part),
				MsgStart:        i,
				MsgEnd:          i,
				AtomSeq:         nextAtomSeq,
				AtomType:        atomType,
				Role:            msg.Role,
			})
			nextContextSeq++
		}
		nextAtomSeq++
		i++
	}
	return nodes
}

func selectCompactFrontierWindow(nodes []FrontierNode, deficitTokens int, maxInputTokens int) compactFrontierSelection {
	if len(nodes) == 0 {
		return compactFrontierSelection{}
	}
	protectedAtomSeq := nodes[len(nodes)-1].AtomSeq
	eligibleEnd := len(nodes)
	for i := range nodes {
		if nodes[i].AtomSeq == protectedAtomSeq {
			eligibleEnd = i
			break
		}
	}
	if eligibleEnd <= 0 {
		return compactFrontierSelection{}
	}

	targetTokens := int(math.Ceil(float64(deficitTokens) / 0.8))
	if targetTokens < 1024 {
		targetTokens = 1024
	}
	selection := compactFrontierSelection{
		EndNodeIndex: -1,
		TargetTokens: targetTokens,
	}
	for i := 0; i < eligibleEnd; i++ {
		selection.Nodes = append(selection.Nodes, nodes[i])
		selection.EndNodeIndex = i
		selection.SelectedTokens += nodes[i].ApproxTokens
		if selection.SelectedTokens >= targetTokens {
			break
		}
	}
	if selection.EndNodeIndex < 0 {
		return compactFrontierSelection{}
	}
	if last := selection.Nodes[len(selection.Nodes)-1]; last.AtomType == compactAtomToolEpisode {
		for i := selection.EndNodeIndex + 1; i < eligibleEnd && nodes[i].AtomSeq == last.AtomSeq; i++ {
			selection.Nodes = append(selection.Nodes, nodes[i])
			selection.EndNodeIndex = i
			selection.SelectedTokens += nodes[i].ApproxTokens
		}
	}
	for selection.EndNodeIndex >= 0 && selection.SelectedTokens > maxInputTokens {
		last := selection.Nodes[len(selection.Nodes)-1]
		cut := 1
		if last.AtomType == compactAtomToolEpisode {
			for cut < len(selection.Nodes) && selection.Nodes[len(selection.Nodes)-1-cut].AtomSeq == last.AtomSeq {
				cut++
			}
		}
		for i := 0; i < cut; i++ {
			selection.SelectedTokens -= selection.Nodes[len(selection.Nodes)-1].ApproxTokens
			selection.Nodes = selection.Nodes[:len(selection.Nodes)-1]
			selection.EndNodeIndex--
		}
	}
	if len(selection.Nodes) == 0 {
		return compactFrontierSelection{}
	}
	if selection.EndNodeIndex+1 < len(nodes) && nodes[selection.EndNodeIndex+1].AtomSeq == nodes[selection.EndNodeIndex].AtomSeq {
		selection.PartialTail = nodes[selection.EndNodeIndex].AtomType != compactAtomToolEpisode
	}
	return selection
}

func buildCompactSummaryInputFromNodes(nodes []FrontierNode) string {
	if len(nodes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		text := strings.TrimSpace(node.SourceText)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func runContextCompactLLMForNodes(
	ctx context.Context,
	rc *RunContext,
	gateway llm.Gateway,
	model string,
	nodes []FrontierNode,
) (string, error) {
	if gateway == nil || strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("gateway or model missing")
	}
	targetText := buildCompactSummaryInputFromNodes(nodes)
	if targetText == "" {
		return "", nil
	}
	runes := []rune(targetText)
	if len(runes) > contextCompactMaxLLMInputRunes {
		targetText = string(runes[:contextCompactMaxLLMInputRunes])
	}
	var userBlock strings.Builder
	userBlock.WriteString("<target-chunks>\n")
	userBlock.WriteString(targetText)
	userBlock.WriteString("\n</target-chunks>\n\n")
	userBlock.WriteString(contextCompactInitialPrompt)

	maxTok := contextCompactMaxOut
	req := llm.Request{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: []llm.TextPart{{Text: compactSystemPromptForRun(ctx, rc, contextCompactSystemPrompt, nil)}}},
			{Role: "user", Content: []llm.TextPart{{Text: userBlock.String()}}},
		},
		MaxOutputTokens: &maxTok,
	}
	streamCtx, cancel := context.WithTimeout(ctx, contextCompactStreamTimeout)
	defer cancel()

	var chunks []string
	err := gateway.Stream(streamCtx, req, func(ev llm.StreamEvent) error {
		switch typed := ev.(type) {
		case llm.StreamMessageDelta:
			if typed.Channel != nil && *typed.Channel == "thinking" {
				return nil
			}
			if typed.ContentDelta != "" {
				chunks = append(chunks, typed.ContentDelta)
			}
		case llm.StreamRunCompleted:
			return errContextCompactStreamDone
		case llm.StreamRunFailed:
			return fmt.Errorf("stream failed: %s", typed.Error.Message)
		}
		return nil
	})
	if err != nil && !errors.Is(err, errContextCompactStreamDone) {
		return "", err
	}
	return strings.TrimSpace(strings.Join(chunks, "")), nil
}

func compactNodesWithShrinkRetry(
	ctx context.Context,
	rc *RunContext,
	gateway llm.Gateway,
	model string,
	nodes []FrontierNode,
) (string, []FrontierNode, error) {
	if len(nodes) == 0 {
		return "", nil, nil
	}
	current := append([]FrontierNode(nil), nodes...)
	for len(current) > 0 {
		summary, err := runContextCompactLLMForNodes(ctx, rc, gateway, model, current)
		if err == nil {
			return summary, current, nil
		}
		if !isContextWindowExceeded(strings.ToLower(err.Error())) || len(current) == 1 {
			return "", current, err
		}
		last := current[len(current)-1]
		cut := 1
		if last.AtomType == compactAtomToolEpisode {
			for cut < len(current) && current[len(current)-1-cut].AtomSeq == last.AtomSeq {
				cut++
			}
		}
		current = current[:len(current)-cut]
	}
	return "", nil, nil
}

func shrinkFrontierNodesToMessageBoundary(nodes []FrontierNode) []FrontierNode {
	current := append([]FrontierNode(nil), nodes...)
	for len(current) > 0 {
		last := current[len(current)-1]
		if last.AtomType == compactAtomToolEpisode {
			return current
		}
		if len(current) == 1 {
			return current
		}
		if current[len(current)-2].MsgStart != last.MsgStart {
			return current
		}
		current = current[:len(current)-1]
	}
	return nil
}

func compactNodesWithPersistRetry(
	ctx context.Context,
	rc *RunContext,
	gateway llm.Gateway,
	model string,
	nodes []FrontierNode,
) (string, []FrontierNode, error) {
	current := shrinkFrontierNodesToMessageBoundary(nodes)
	for len(current) > 0 {
		summary, usedNodes, err := compactNodesWithShrinkRetry(ctx, rc, gateway, model, current)
		if err != nil {
			return "", usedNodes, err
		}
		bounded := shrinkFrontierNodesToMessageBoundary(usedNodes)
		if len(bounded) == 0 {
			return "", nil, nil
		}
		if len(bounded) != len(usedNodes) {
			current = bounded
			continue
		}
		return summary, bounded, nil
	}
	return "", nil, nil
}

func isContextWindowExceeded(errMsg string) bool {
	for _, kw := range []string{"context_length_exceeded", "max_tokens", "too many tokens", "maximum context length", "token limit"} {
		if strings.Contains(errMsg, kw) {
			return true
		}
	}
	return false
}

func materializeCompactedPrefixMessages(
	msgs []llm.Message,
	nodes []FrontierNode,
	endNodeIndex int,
	summary string,
) []llm.Message {
	if len(msgs) == 0 || len(nodes) == 0 || endNodeIndex < 0 || endNodeIndex >= len(nodes) {
		return msgs
	}
	out := make([]llm.Message, 0, len(msgs)+1)
	out = append(out, makeThreadContextReplacementMessage(summary))

	endNode := nodes[endNodeIndex]
	nextNodeIndex := endNodeIndex + 1
	if nextNodeIndex < len(nodes) && nodes[nextNodeIndex].MsgStart == endNode.MsgStart && endNode.AtomType != compactAtomToolEpisode {
		tailParts := make([]string, 0, 4)
		for i := nextNodeIndex; i < len(nodes) && nodes[i].MsgStart == endNode.MsgStart; i++ {
			tailParts = append(tailParts, strings.TrimSpace(nodes[i].SourceText))
		}
		tailText := strings.TrimSpace(strings.Join(tailParts, "\n\n"))
		if tailText != "" {
			tailMsg := llm.Message{
				Role:    msgs[endNode.MsgStart].Role,
				Phase:   msgs[endNode.MsgStart].Phase,
				Content: []llm.ContentPart{{Type: messagecontent.PartTypeText, Text: tailText}},
			}
			out = append(out, tailMsg)
		}
		out = append(out, msgs[endNode.MsgStart+1:]...)
		return out
	}

	out = append(out, msgs[endNode.MsgEnd+1:]...)
	return out
}

func materializeCompactedPrefixIDs(ids []uuid.UUID, nodes []FrontierNode, endNodeIndex int) []uuid.UUID {
	if len(ids) == 0 || len(nodes) == 0 || endNodeIndex < 0 || endNodeIndex >= len(nodes) {
		return ids
	}
	out := make([]uuid.UUID, 0, len(ids)+1)
	out = append(out, uuid.Nil)
	endNode := nodes[endNodeIndex]
	nextNodeIndex := endNodeIndex + 1
	if nextNodeIndex < len(nodes) && nodes[nextNodeIndex].MsgStart == endNode.MsgStart && endNode.AtomType != compactAtomToolEpisode {
		out = append(out, ids[endNode.MsgStart])
		out = append(out, ids[endNode.MsgStart+1:]...)
		return out
	}
	out = append(out, ids[endNode.MsgEnd+1:]...)
	return out
}
