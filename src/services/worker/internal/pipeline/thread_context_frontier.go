package pipeline

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type frontierReplacementCoverage struct {
	record          data.ThreadContextReplacementRecord
	startContextSeq int64
	endContextSeq   int64
	chunkIDs        map[uuid.UUID]struct{}
}

func buildThreadContextFrontier(
	ctx context.Context,
	tx pgx.Tx,
	graph *persistedCanonicalThreadGraph,
	accountID uuid.UUID,
	threadID uuid.UUID,
	replacements []data.ThreadContextReplacementRecord,
	upperBoundContextSeq *int64,
) ([]FrontierNode, error) {
	if graph == nil {
		return nil, nil
	}
	replacementCoverage, err := resolveReplacementCoverage(ctx, tx, graph, accountID, threadID, replacements)
	if err != nil {
		return nil, err
	}
	replacementCoverage = selectPrefixOnlyReplacementCoverage(replacementCoverage, graph, upperBoundContextSeq)

	nodes := make([]FrontierNode, 0, len(graph.Chunks)+len(replacementCoverage))
	replacementIndex := 0
	appendReplacement := func(item frontierReplacementCoverage) {
		text := strings.TrimSpace(item.record.SummaryText)
		if text == "" {
			return
		}
		nodes = append(nodes, FrontierNode{
			Kind:            FrontierNodeReplacement,
			NodeID:          item.record.ID,
			Layer:           item.record.Layer,
			StartContextSeq: item.startContextSeq,
			EndContextSeq:   item.endContextSeq,
			StartThreadSeq:  item.record.StartThreadSeq,
			EndThreadSeq:    item.record.EndThreadSeq,
			SourceText:      text,
			ApproxTokens:    approxTokensFromText(text),
			AtomSeq:         int(item.record.StartContextSeq),
			AtomType:        compactAtomAssistantText,
			role:            "system",
			Role:            "system",
		})
	}

	for _, chunk := range graph.Chunks {
		if upperBoundContextSeq != nil && chunk.ContextSeq > *upperBoundContextSeq {
			break
		}
		for replacementIndex < len(replacementCoverage) && replacementCoverage[replacementIndex].startContextSeq <= chunk.ContextSeq {
			appendReplacement(replacementCoverage[replacementIndex])
			replacementIndex++
		}
		record := graph.ChunkRecordsByContextSeq[chunk.ContextSeq]
		if record == nil {
			return nil, fmt.Errorf("chunk record missing for context_seq=%d", chunk.ContextSeq)
		}
		if replacementIndex > 0 {
			covered := false
			for idx := replacementIndex - 1; idx >= 0; idx-- {
				if _, ok := replacementCoverage[idx].chunkIDs[record.ID]; ok {
					covered = true
					break
				}
				if replacementCoverage[idx].endContextSeq < chunk.ContextSeq {
					break
				}
			}
			if covered {
				continue
			}
		}
		nodes = append(nodes, FrontierNode{
			Kind:            FrontierNodeChunk,
			NodeID:          record.ID,
			StartContextSeq: chunk.ContextSeq,
			EndContextSeq:   chunk.ContextSeq,
			StartThreadSeq:  chunk.StartThreadSeq,
			EndThreadSeq:    chunk.EndThreadSeq,
			SourceText:      strings.TrimSpace(chunk.Content),
			ApproxTokens:    approxTokensFromText(chunk.Content),
			AtomSeq:         frontierChunkAtomSeq(graph, chunk.AtomKey),
			AtomType:        frontierChunkAtomType(graph, chunk.AtomKey),
			Role:            frontierChunkRole(graph, chunk.AtomKey),
			atomKey:         chunk.AtomKey,
			chunkSeq:        chunk.ContextSeq,
			chunkKind:       record.ChunkKind,
		})
	}
	for replacementIndex < len(replacementCoverage) {
		if upperBoundContextSeq == nil || replacementCoverage[replacementIndex].endContextSeq <= *upperBoundContextSeq {
			appendReplacement(replacementCoverage[replacementIndex])
		}
		replacementIndex++
	}
	return nodes, nil
}

func frontierChunkAtomSeq(graph *persistedCanonicalThreadGraph, atomKey string) int {
	if graph == nil {
		return 0
	}
	record := graph.AtomRecordsByKey[atomKey]
	if record == nil {
		return 0
	}
	return int(record.AtomSeq)
}

func frontierChunkRole(graph *persistedCanonicalThreadGraph, atomKey string) string {
	if graph == nil {
		return ""
	}
	record := graph.AtomRecordsByKey[atomKey]
	if record == nil {
		return ""
	}
	return strings.TrimSpace(record.Role)
}

func frontierChunkAtomType(graph *persistedCanonicalThreadGraph, atomKey string) compactAtomType {
	if graph == nil {
		return compactAtomAssistantText
	}
	record := graph.AtomRecordsByKey[atomKey]
	if record == nil {
		return compactAtomAssistantText
	}
	switch strings.TrimSpace(record.AtomKind) {
	case string(canonicalAtomUserText):
		return compactAtomUserText
	case string(canonicalAtomToolEpisode):
		return compactAtomToolEpisode
	default:
		return compactAtomAssistantText
	}
}

func selectPrefixOnlyReplacementCoverage(
	items []frontierReplacementCoverage,
	graph *persistedCanonicalThreadGraph,
	upperBoundContextSeq *int64,
) []frontierReplacementCoverage {
	if len(items) == 0 || graph == nil || len(graph.Chunks) == 0 {
		return nil
	}
	candidates := append([]frontierReplacementCoverage(nil), items...)
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].startContextSeq != candidates[j].startContextSeq {
			return candidates[i].startContextSeq < candidates[j].startContextSeq
		}
		if candidates[i].record.Layer != candidates[j].record.Layer {
			return candidates[i].record.Layer > candidates[j].record.Layer
		}
		if !candidates[i].record.CreatedAt.Equal(candidates[j].record.CreatedAt) {
			return candidates[i].record.CreatedAt.After(candidates[j].record.CreatedAt)
		}
		if candidates[i].endContextSeq != candidates[j].endContextSeq {
			return candidates[i].endContextSeq < candidates[j].endContextSeq
		}
		return candidates[i].record.ID.String() < candidates[j].record.ID.String()
	})

	firstContextSeq := graph.Chunks[0].ContextSeq
	selected := make([]frontierReplacementCoverage, 0, len(candidates))
	expectedStart := firstContextSeq
	for {
		bestIndex := -1
		for idx, candidate := range candidates {
			if upperBoundContextSeq != nil && candidate.endContextSeq > *upperBoundContextSeq {
				continue
			}
			if candidate.startContextSeq < expectedStart {
				continue
			}
			if candidate.startContextSeq > expectedStart {
				break
			}
			bestIndex = idx
			break
		}
		if bestIndex < 0 {
			break
		}
		selected = append(selected, candidates[bestIndex])
		expectedStart = candidates[bestIndex].endContextSeq + 1
	}
	return selected
}

func resolveReplacementCoverage(
	ctx context.Context,
	tx pgx.Tx,
	graph *persistedCanonicalThreadGraph,
	accountID uuid.UUID,
	threadID uuid.UUID,
	replacements []data.ThreadContextReplacementRecord,
) ([]frontierReplacementCoverage, error) {
	if len(replacements) == 0 || graph == nil {
		return nil, nil
	}
	replacementsRepo := data.ThreadContextReplacementsRepository{}
	edgesRepo := data.ThreadContextSupersessionEdgesRepository{}
	threadEdges, err := edgesRepo.ListByThread(ctx, tx, accountID, threadID)
	if err != nil {
		return nil, err
	}
	edgesByReplacementID := make(map[uuid.UUID][]data.ThreadContextSupersessionEdgeRecord, len(replacements))
	for _, edge := range threadEdges {
		edgesByReplacementID[edge.ReplacementID] = append(edgesByReplacementID[edge.ReplacementID], edge)
	}
	replacementCache := make(map[uuid.UUID]*data.ThreadContextReplacementRecord, len(replacements))
	for i := range replacements {
		item := replacements[i]
		replacementCache[item.ID] = &item
	}
	chunkRangeByReplacementID := make(map[uuid.UUID]frontierReplacementCoverage, len(replacements))
	var expand func(uuid.UUID, map[uuid.UUID]struct{}) (frontierReplacementCoverage, error)
	expand = func(replacementID uuid.UUID, visiting map[uuid.UUID]struct{}) (frontierReplacementCoverage, error) {
		if cached, ok := chunkRangeByReplacementID[replacementID]; ok {
			return cached, nil
		}
		if _, ok := visiting[replacementID]; ok {
			return frontierReplacementCoverage{}, fmt.Errorf("replacement supersession cycle detected: %s", replacementID)
		}
		record := replacementCache[replacementID]
		if record == nil {
			loaded, err := replacementsRepo.GetByID(ctx, tx, accountID, threadID, replacementID)
			if err != nil {
				return frontierReplacementCoverage{}, err
			}
			if loaded == nil {
				return frontierReplacementCoverage{}, fmt.Errorf("replacement not found: %s", replacementID)
			}
			replacementCache[replacementID] = loaded
			record = loaded
		}
		edges := edgesByReplacementID[replacementID]
		chunkIDs := make(map[uuid.UUID]struct{})
		visiting[replacementID] = struct{}{}
		for _, edge := range edges {
			if edge.SupersededChunkID != nil && *edge.SupersededChunkID != uuid.Nil {
				chunkIDs[*edge.SupersededChunkID] = struct{}{}
				continue
			}
			if edge.SupersededReplacementID != nil && *edge.SupersededReplacementID != uuid.Nil {
				nested, err := expand(*edge.SupersededReplacementID, visiting)
				if err != nil {
					return frontierReplacementCoverage{}, err
				}
				for chunkID := range nested.chunkIDs {
					chunkIDs[chunkID] = struct{}{}
				}
			}
		}
		delete(visiting, replacementID)
		if len(chunkIDs) == 0 {
			return frontierReplacementCoverage{}, nil
		}
		startContextSeq := int64(0)
		endContextSeq := int64(0)
		for chunkID := range chunkIDs {
			record := graph.ChunkRecordsByID[chunkID]
			if record == nil {
				return frontierReplacementCoverage{}, fmt.Errorf("chunk record missing for replacement %s target %s", replacementID, chunkID)
			}
			if startContextSeq == 0 || record.ContextSeq < startContextSeq {
				startContextSeq = record.ContextSeq
			}
			if record.ContextSeq > endContextSeq {
				endContextSeq = record.ContextSeq
			}
		}
		item := frontierReplacementCoverage{
			record:          *record,
			startContextSeq: startContextSeq,
			endContextSeq:   endContextSeq,
			chunkIDs:        chunkIDs,
		}
		chunkRangeByReplacementID[replacementID] = item
		return item, nil
	}

	out := make([]frontierReplacementCoverage, 0, len(replacements))
	for _, item := range replacements {
		resolved, err := expand(item.ID, map[uuid.UUID]struct{}{})
		if err != nil {
			return nil, err
		}
		if len(resolved.chunkIDs) == 0 {
			continue
		}
		out = append(out, resolved)
	}
	return out, nil
}
