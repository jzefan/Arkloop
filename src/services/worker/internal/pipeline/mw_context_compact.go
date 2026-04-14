package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/routing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pkoukk/tiktoken-go"
)

const (
	settingContextCompactionModel  = "context.compaction.model"
	contextCompactStreamTimeout    = 60 * time.Second
	contextCompactTimeBudget       = 90 * time.Second
	contextCompactMaxOut           = 4096
	contextCompactGroupMaxOut      = 8192
	contextCompactPostWriteTimeout = 30 * time.Second
	defaultPersistKeepLastMessages = 40
	// 发往压缩摘要 LLM 的用户块上限（tiktoken 用 HistoryThreadPromptTokens；单条超大时再按 rune 截断）。
	contextCompactMaxLLMInputTokens = 120000
	contextCompactMaxLLMInputRunes  = 400000
	// 快速裁切：已有 snapshot 且待压缩前缀消息不超过此数量时，跳过 LLM 直接复用已有摘要
	fastCompactMaxPrefixMessages = 4
)

const contextCompactSystemPrompt = `You are a dialogue compression assistant.

Compress the conversation faithfully so another model can continue with minimal loss.

Rules:
- This is compression, not analysis.
- Do NOT infer goals, plans, blockers, or "next steps" unless they were explicitly said.
- Preserve concrete facts, chronology, unresolved questions, decisions actually stated, file paths, function names, commands, URLs, numbers, errors, IDs, and quoted wording when important.
- Remove filler, repetition, greetings, and other low-information chatter.
- Keep the output in the dominant language of the conversation.
- Output only the compressed conversation text.`

const contextCompactInitialPrompt = `Rewrite the content in <target-chunks> into a shorter faithful version.

Output rules:
- Keep chronological order.
- Use short bullet points.
- Mention the speaker only when it helps disambiguate.
- Preserve concrete details exactly when they matter.
- Do not turn the conversation into a project report or task analysis.
- Do not add headings such as Goal, Progress, Next Steps, or Decisions unless those words were part of the original conversation.
- Do not answer the conversation.`

var errContextCompactStreamDone = errors.New("context_compact_stream_done")

// NewContextCompactMiddleware 在 TitleSummarizer 之后运行：可选将头部区间摘要持久化，再按预算裁切消息。
func NewContextCompactMiddleware(
	pool CompactPersistDB,
	messagesRepo data.MessagesRepository,
	eventsRepo CompactRunEventAppender,
	auxGateway llm.Gateway,
	emitDebugEvents bool,
	loaders ...*routing.ConfigLoader,
) RunMiddleware {
	var configLoader *routing.ConfigLoader
	if len(loaders) > 0 {
		configLoader = loaders[0]
	}
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		beforeMsgs := append([]llm.Message(nil), rc.Messages...)
		cfg := rc.ContextCompact
		if rewritten, stripped := stripOlderImagePartsKeepingTail(rc.Messages, resolveContextKeepImageTail()); stripped > 0 {
			rc.Messages = rewritten
		}
		if !cfg.Enabled && !cfg.PersistEnabled {
			beforeTokens := traceContextCompactTokens(nil, rc.SystemPrompt, beforeMsgs)
			afterTokens := traceContextCompactTokens(nil, rc.SystemPrompt, rc.Messages)
			emitTraceEvent(rc, "context_compact", "context_compact.completed", map[string]any{
				"compacted":     beforeTokens != afterTokens || len(beforeMsgs) != len(rc.Messages),
				"tokens_before": beforeTokens,
				"tokens_after":  afterTokens,
			})
			return next(ctx, rc)
		}

		var enc *tiktoken.Tiktoken
		if rc.SelectedRoute != nil {
			if tke, encErr := ResolveTiktokenForRoute(rc.SelectedRoute); encErr != nil {
				slog.WarnContext(ctx, "context_compact", "phase", "tiktoken_route", "err", encErr.Error(), "run_id", rc.Run.ID.String())
			} else {
				enc = tke
			}
		}
		if enc == nil {
			enc, _ = tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
		}
		beforeTokens := traceContextCompactTokens(enc, rc.SystemPrompt, beforeMsgs)

		if cfg.MicrocompactKeepRecentTools > 0 {
			rc.Messages = microcompactToolResults(rc.Messages, cfg.MicrocompactKeepRecentTools)
		}

		beforeN := len(rc.Messages)
		msgs := rc.Messages
		ids := rc.ThreadMessageIDs
		persistSplit := 0
		pendingPersists := make([]pendingPersistCompaction, 0, 4)

		if cfg.PersistEnabled && pool != nil && rc.Gateway != nil && len(msgs) > 1 {
			window := 0
			if rc.SelectedRoute != nil {
				window = routing.RouteContextWindowTokens(rc.SelectedRoute.Route)
			}
			trigger, window := compactPersistTriggerTokens(cfg, window)
			keep := cfg.PersistKeepLastMessages
			if keep <= 0 {
				keep = defaultPersistKeepLastMessages
			}
			requestEstimate := HistoryThreadPromptTokens(enc, contextCompactRequestMessages(rc.SystemPrompt, msgs))
			anchor, anchored := resolveContextCompactPressureAnchor(ctx, pool, rc)
			if anchored {
				rc.SetContextCompactPressureAnchor(anchor.LastRealPromptTokens, anchor.LastRequestContextEstimateTokens)
			}
			pressure := ComputeContextCompactPressure(requestEstimate, func() *ContextCompactPressureAnchor {
				if !anchored {
					return nil
				}
				return &anchor
			}())
			if pressure.ContextPressureTokens >= trigger && len(ids) == len(msgs) {
				// 断路器：连续失败过多则跳过 persist
				if pool != nil && compactConsecutiveFailures(ctx, pool, rc.Run.AccountID, rc.Run.ThreadID) >= maxConsecutiveCompactFailures {
					slog.WarnContext(ctx, "context_compact", "phase", "circuit_breaker", "run_id", rc.Run.ID.String(), "thread_id", rc.Run.ThreadID.String())
					circuitBreakerEvent := map[string]any{
						"op":    "persist",
						"phase": "circuit_breaker",
					}
					ApplyContextCompactPressureFields(circuitBreakerEvent, pressure)
					if evErr := appendContextCompactRunEvent(ctx, pool, eventsRepo, rc, circuitBreakerEvent); evErr != nil {
						slog.WarnContext(ctx, "context_compact", "phase", "circuit_breaker_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
					}
				} else {
					compactBase := msgs
					compactBaseIDs := ids
					evaluatingEvent := map[string]any{
						"op":              "persist",
						"phase":           "evaluating",
						"pressure_tokens": pressure.ContextPressureTokens,
						"trigger_tokens":  trigger,
						"window_tokens":   window,
						"message_count":   len(msgs),
					}
					if evErr := appendContextCompactRunEvent(ctx, pool, eventsRepo, rc, evaluatingEvent); evErr != nil {
						slog.WarnContext(ctx, "context_compact", "phase", "evaluating_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
					}
					persistFrontier := append([]FrontierNode(nil), rc.ThreadContextFrontier...)
					if len(persistFrontier) == 0 {
						persistFrontier = buildCompactFrontierAtomsFromMessages(enc, compactBase)
					}
					targetTokens := contextCompactTargetTokens(cfg, window)
					if targetTokens <= 0 {
						targetTokens = trigger
					}
					if targetTokens >= pressure.ContextPressureTokens {
						targetTokens = trigger - 1
					}
					if targetTokens < 1 {
						targetTokens = 1
					}
					gw, model := resolveCompactionGateway(ctx, pool, rc, auxGateway, emitDebugEvents, configLoader)
					if gw == nil {
						slog.WarnContext(ctx, "context_compact", "phase", "gateway_nil", "run_id", rc.Run.ID.String())
					} else {
						var fileLockCleanup func()
						var fileLockErr error
						if pool != nil {
							fileLockCleanup, fileLockErr = CompactThreadCompactionLock(ctx, rc.Run.ThreadID)
							if fileLockErr != nil {
								slog.WarnContext(ctx, "context_compact", "phase", "file_lock", "err", fileLockErr.Error(), "run_id", rc.Run.ID.String())
							}
							if fileLockCleanup != nil {
								defer fileLockCleanup()
							}
						}

						compactDeadline := time.Now().Add(contextCompactTimeBudget)
						persistRound := 0
						for pressure.ContextPressureTokens > targetTokens {
							if time.Now().After(compactDeadline) {
								slog.WarnContext(ctx, "context_compact", "phase", "persist_time_budget_exceeded", "run_id", rc.Run.ID.String(), "round", persistRound)
								break
							}
							persistRound++

							selectionFrontier := buildCompactFrontierAtomsFromPersistFrontier(persistFrontier)
							if len(selectionFrontier) == 0 {
								selectionFrontier = buildCompactFrontierAtomsFromMessages(enc, compactBase)
							}
							selection := selectCompactAtomWindow(selectionFrontier, pressure.ContextPressureTokens-targetTokens, contextCompactMaxLLMInputTokens)
							if len(selection.Nodes) == 0 {
								break
							}

							startedEvent := map[string]any{
								"op":                    "persist",
								"mode":                  "canonical_atoms",
								"phase":                 "round_started",
								"round":                 persistRound,
								"atoms_selected":        len(selection.Nodes),
								"selected_tokens":       selection.SelectedTokens,
								"persist_split":         selection.EndNodeIndex + 1,
								"trigger_tokens":        trigger,
								"context_window_tokens": window,
								"trigger_context_pct":   cfg.PersistTriggerContextPct,
								"tail_keep_effective":   keep,
							}
							ApplyContextCompactPressureFields(startedEvent, pressure)
							if evErr := appendContextCompactRunEvent(ctx, pool, eventsRepo, rc, startedEvent); evErr != nil {
								slog.WarnContext(ctx, "context_compact", "phase", "round_started_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
							}

							targetNodes := selection.Nodes
							targetNodeCount := len(targetNodes)
							roundMessagesBefore := len(compactBase)
							beforeRoundEstimate := requestEstimate
							roundInputMessages := append([]llm.Message(nil), compactBase...)
							progress := newCompactProgressRecorder(pool, eventsRepo, map[string]any{
								"op":    "persist",
								"mode":  "canonical_atoms",
								"round": persistRound,
							})
							summary, usedNodes, sumErr := compactNodesWithPersistRetry(ctx, rc, gw, model, targetNodes, progress)
							if sumErr != nil {
								slog.WarnContext(ctx, "context_compact", "phase", "llm", "err", sumErr.Error(), "run_id", rc.Run.ID.String(), "round", persistRound)
								failedEvent := map[string]any{
									"op":                 "persist",
									"mode":               "canonical_atoms",
									"phase":              "llm_failed",
									"round":              persistRound,
									"persist_split":      selection.EndNodeIndex + 1,
									"llm_error":          sumErr.Error(),
									"trigger_tokens":     trigger,
									"target_chunk_count": targetNodeCount,
								}
								ApplyContextCompactPressureFields(failedEvent, pressure)
								if evErr := appendContextCompactRunEvent(ctx, pool, eventsRepo, rc, failedEvent); evErr != nil {
									slog.WarnContext(ctx, "context_compact", "phase", "llm_failed_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
								}
								break
							}

							summary = strings.TrimSpace(summary)
							if summary == "" || len(usedNodes) == 0 {
								break
							}

							persistNodes := mapSelectedAtomsToPersistFrontierNodes(usedNodes, persistFrontier)
							if len(persistNodes) == 0 {
								persistNodes = append([]FrontierNode(nil), usedNodes...)
							}

							lastUsed := usedNodes[len(usedNodes)-1]
							persistSplit = lastUsed.MsgEnd + 1
							persistPrefixIDs := append([]uuid.UUID(nil), filterNonNilUUIDs(compactBaseIDs[:persistSplit])...)
							roundCompletedEvent := map[string]any{
								"op":                    "persist",
								"mode":                  "canonical_atoms",
								"phase":                 "round_completed",
								"round":                 persistRound,
								"persist_split":         persistSplit,
								"context_window_tokens": window,
								"trigger_tokens":        trigger,
								"trigger_context_pct":   cfg.PersistTriggerContextPct,
								"tail_keep_effective":   keep,
								"summary_tokens":        compactTokenCount(enc, summary),
								"atoms_compacted":       len(usedNodes),
							}
							ApplyContextCompactPressureFields(roundCompletedEvent, pressure)
							if evErr := appendContextCompactRunEvent(ctx, pool, eventsRepo, rc, roundCompletedEvent); evErr != nil {
								slog.WarnContext(ctx, "context_compact", "phase", "round_completed_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
							}

							placeholderReplacementID := compactFrontierNodeID(FrontierNodeReplacement, persistRound, persistSplit, persistSplit, len(pendingPersists))
							completedEvent := map[string]any{
								"op":                    "persist",
								"mode":                  "canonical_atoms",
								"phase":                 "completed",
								"round":                 persistRound,
								"persist_split":         persistSplit,
								"messages_before":       roundMessagesBefore,
								"context_window_tokens": window,
								"trigger_tokens":        trigger,
								"trigger_context_pct":   cfg.PersistTriggerContextPct,
								"tail_keep_configured":  keep,
								"tail_keep_effective":   keep,
								"target_chunk_count":    targetNodeCount,
							}
							ApplyContextCompactPressureFields(completedEvent, pressure)
							pendingPersists = append(pendingPersists, pendingPersistCompaction{
								PlaceholderReplacementID: placeholderReplacementID,
								Summary:                  summary,
								WindowNodes:              append([]FrontierNode(nil), persistNodes...),
								PrefixIDs:                persistPrefixIDs,
								CompletedEvent:           completedEvent,
							})

							msgs = materializeCompactedPrefixAtoms(compactBase, usedNodes, len(usedNodes)-1, summary)
							msgs = truncateLargeTailMessages(enc, msgs)
							ids = materializeCompactedPrefixIDs(compactBaseIDs, usedNodes, len(usedNodes)-1)
							persistFrontier = materializeCompactedPrefixFrontier(persistFrontier, persistNodes, summary, placeholderReplacementID)
							compactBase = msgs
							compactBaseIDs = ids
							requestEstimate = HistoryThreadPromptTokens(enc, contextCompactRequestMessages(rc.SystemPrompt, compactBase))
							pressure = ComputeContextCompactPressure(requestEstimate, func() *ContextCompactPressureAnchor {
								if !anchored {
									return nil
								}
								return &anchor
							}())
							rc.Messages = msgs
							rc.ThreadMessageIDs = ids
							rc.ThreadContextFrontier = append([]FrontierNode(nil), persistFrontier...)
							systemPrompt := compactSystemPromptForRun(ctx, rc, contextCompactSystemPrompt, nil)
							notifyCompactApplied(ctx, rc, CompactInput{
								SystemPrompt: systemPrompt,
								Messages:     roundInputMessages,
							}, CompactOutput{
								SystemPrompt: systemPrompt,
								Messages:     append([]llm.Message(nil), rc.Messages...),
								Summary:      summary,
								Changed:      true,
							})
							if requestEstimate >= beforeRoundEstimate {
								break
							}
						}
					}
				}
			}
		}

		var trimEvent map[string]any
		if cfg.Enabled && ContextCompactHasActiveBudget(cfg) {
			beforeTrim := len(rc.Messages)
			beforeTrimTok := HistoryThreadPromptTokens(enc, rc.Messages)
			out, outIDs, dropped := CompactThreadMessages(rc.Messages, rc.ThreadMessageIDs, cfg, enc)
			rc.Messages = out
			rc.ThreadMessageIDs = outIDs
			if dropped > 0 || len(out) != beforeTrim {
				slog.InfoContext(ctx, "context_compact",
					"run_id", rc.Run.ID.String(),
					"thread_id", rc.Run.ThreadID.String(),
					"phase", "trim",
					"dropped_prefix", dropped,
					"after", len(out),
				)
				trimEvent = map[string]any{
					"op":                            "trim",
					"phase":                         "completed",
					"dropped_prefix":                dropped,
					"messages_before":               beforeTrim,
					"messages_after":                len(out),
					"thread_tokens_tiktoken_before": beforeTrimTok,
					"thread_tokens_tiktoken_after":  HistoryThreadPromptTokens(enc, out),
				}
			}
		}

		if persistSplit > 0 {
			slog.InfoContext(ctx, "context_compact",
				"run_id", rc.Run.ID.String(),
				"thread_id", rc.Run.ThreadID.String(),
				"phase", "persist",
				"persist_split", persistSplit,
				"before", beforeN,
				"after", len(rc.Messages),
			)
		}
		if cfg.PersistEnabled && pool != nil {
			middlewareCompletedEvent := map[string]any{
				"op":              "persist",
				"phase":           "middleware_completed",
				"persist_applied": persistSplit > 0,
				"message_count":   len(rc.Messages),
			}
			if evErr := appendContextCompactRunEvent(ctx, pool, eventsRepo, rc, middlewareCompletedEvent); evErr != nil {
				slog.WarnContext(ctx, "context_compact", "phase", "middleware_completed_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
			}
		}

		nextErr := next(ctx, rc)

		postCtx, cancel := context.WithTimeout(context.Background(), contextCompactPostWriteTimeout)
		defer cancel()

		if pool != nil && len(pendingPersists) > 0 {
			insertedReplacementIDs := make(map[uuid.UUID]uuid.UUID, len(pendingPersists))
			for _, pending := range pendingPersists {
				if strings.TrimSpace(pending.Summary) == "" || len(pending.WindowNodes) == 0 {
					continue
				}
				tx, txErr := pool.BeginTx(postCtx, pgx.TxOptions{})
				if txErr != nil {
					slog.WarnContext(ctx, "context_compact", "phase", "tx_begin", "err", txErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				if lockErr := compactThreadCompactionAdvisoryXactLock(postCtx, tx, rc.Run.ThreadID); lockErr != nil {
					_ = tx.Rollback(postCtx)
					emitContextCompactFailure(ctx, postCtx, pool, eventsRepo, rc, "persist", "advisory_lock", lockErr)
					slog.WarnContext(ctx, "context_compact", "phase", "advisory_lock", "err", lockErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				still, chkErr := compactPrefixMessagesStillAvailable(postCtx, tx, rc.Run.AccountID, rc.Run.ThreadID, pending.PrefixIDs)
				if chkErr != nil {
					_ = tx.Rollback(postCtx)
					emitContextCompactFailure(ctx, postCtx, pool, eventsRepo, rc, "persist", "prefix_precheck", chkErr)
					slog.WarnContext(ctx, "context_compact", "phase", "prefix_precheck", "err", chkErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				if !still {
					_ = tx.Rollback(postCtx)
					continue
				}
				persistPlan, ok, rangeErr := resolvePersistReplacementPlan(
					postCtx,
					tx,
					rc.Run.AccountID,
					rc.Run.ThreadID,
					pending.WindowNodes,
				)
				if rangeErr != nil {
					_ = tx.Rollback(postCtx)
					emitContextCompactFailure(ctx, postCtx, pool, eventsRepo, rc, "persist", "range_resolve", rangeErr)
					slog.WarnContext(ctx, "context_compact", "phase", "range_resolve", "err", rangeErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				if !ok {
					_ = tx.Rollback(postCtx)
					continue
				}
				persistPlan = remapPersistReplacementPlan(persistPlan, insertedReplacementIDs)

				replacementsRepo := data.ThreadContextReplacementsRepository{}
				replacement, insErr := replacementsRepo.Insert(postCtx, tx, data.ThreadContextReplacementInsertInput{
					AccountID:       rc.Run.AccountID,
					ThreadID:        rc.Run.ThreadID,
					StartThreadSeq:  persistPlan.StartThreadSeq,
					EndThreadSeq:    persistPlan.EndThreadSeq,
					StartContextSeq: persistPlan.StartContextSeq,
					EndContextSeq:   persistPlan.EndContextSeq,
					SummaryText:     pending.Summary,
					Layer:           persistPlan.Layer,
					MetadataJSON:    compactReplacementMetadata("context_compact"),
				})
				if insErr != nil {
					_ = tx.Rollback(postCtx)
					emitContextCompactFailure(ctx, postCtx, pool, eventsRepo, rc, "persist", "insert_replacement", insErr)
					slog.WarnContext(ctx, "context_compact", "phase", "insert_replacement", "err", insErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				if edgeErr := writeReplacementSupersessionEdges(postCtx, tx, rc.Run.AccountID, rc.Run.ThreadID, replacement.ID, persistPlan); edgeErr != nil {
					_ = tx.Rollback(postCtx)
					emitContextCompactFailure(ctx, postCtx, pool, eventsRepo, rc, "persist", "write_replacement_edges", edgeErr)
					slog.WarnContext(ctx, "context_compact", "phase", "write_replacement_edges", "err", edgeErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				if supErr := replacementsRepo.SupersedeActiveOverlapsByContextSeq(postCtx, tx, rc.Run.AccountID, rc.Run.ThreadID, replacement.StartContextSeq, replacement.EndContextSeq, replacement.ID); supErr != nil {
					_ = tx.Rollback(postCtx)
					emitContextCompactFailure(ctx, postCtx, pool, eventsRepo, rc, "persist", "supersede_replacements", supErr)
					slog.WarnContext(ctx, "context_compact", "phase", "supersede_replacements", "err", supErr.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				evOk := true
				if pending.CompletedEvent != nil && eventsRepo != nil {
					ev := rc.Emitter.Emit("run.context_compact", pending.CompletedEvent, nil, nil)
					if _, evErr := eventsRepo.AppendRunEvent(postCtx, tx, rc.Run.ID, ev); evErr != nil {
						_ = tx.Rollback(postCtx)
						evOk = false
						slog.WarnContext(ctx, "context_compact", "phase", "run_event", "err", evErr.Error(), "run_id", rc.Run.ID.String())
					}
				}
				if !evOk {
					continue
				}
				if err := tx.Commit(postCtx); err != nil {
					slog.WarnContext(ctx, "context_compact", "phase", "tx_commit", "err", err.Error(), "run_id", rc.Run.ID.String())
					continue
				}
				insertedReplacementIDs[pending.PlaceholderReplacementID] = replacement.ID
			}
		}

		if trimEvent != nil {
			if err := appendContextCompactRunEvent(postCtx, pool, eventsRepo, rc, trimEvent); err != nil {
				slog.WarnContext(ctx, "context_compact", "phase", "run_event_trim", "err", err.Error(), "run_id", rc.Run.ID.String())
			}
		}
		afterTokens := traceContextCompactTokens(enc, rc.SystemPrompt, rc.Messages)
		emitTraceEvent(rc, "context_compact", "context_compact.completed", map[string]any{
			"compacted":     beforeTokens != afterTokens || len(beforeMsgs) != len(rc.Messages),
			"tokens_before": beforeTokens,
			"tokens_after":  afterTokens,
		})

		return nextErr
	}
}

func traceContextCompactTokens(enc *tiktoken.Tiktoken, systemPrompt string, msgs []llm.Message) int {
	if enc == nil {
		enc, _ = tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	}
	return HistoryThreadPromptTokens(enc, contextCompactRequestMessages(systemPrompt, msgs))
}

func filterNonNilUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id != uuid.Nil {
			out = append(out, id)
		}
	}
	return out
}

func clampPersistSplitBeforeSyntheticTail(msgs []llm.Message, ids []uuid.UUID, split int) int {
	if split <= 0 || len(ids) != len(msgs) {
		return split
	}
	leadingPrefix := leadingCompactPrefixMessageCount(msgs, ids)
	for i := leadingPrefix; i < split; i++ {
		if ids[i] == uuid.Nil {
			return i
		}
	}
	return split
}

type persistReplacementPlan struct {
	StartThreadSeq           int64
	EndThreadSeq             int64
	StartContextSeq          int64
	EndContextSeq            int64
	Layer                    int
	SupersededReplacementIDs []uuid.UUID
	SupersededChunkIDs       []uuid.UUID
}

type pendingPersistCompaction struct {
	PlaceholderReplacementID uuid.UUID
	Summary                  string
	WindowNodes              []FrontierNode
	PrefixIDs                []uuid.UUID
	CompletedEvent           map[string]any
}

func resolvePersistReplacementPlan(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	nodes []FrontierNode,
) (persistReplacementPlan, bool, error) {
	if tx == nil {
		return persistReplacementPlan{}, false, fmt.Errorf("tx must not be nil")
	}
	_ = ctx
	plan := persistReplacementPlan{Layer: 1}
	mergeRange := func(startThreadSeq, endThreadSeq, startContextSeq, endContextSeq int64) {
		if startThreadSeq <= 0 || endThreadSeq <= 0 || startThreadSeq > endThreadSeq {
			return
		}
		if startContextSeq <= 0 || endContextSeq <= 0 || startContextSeq > endContextSeq {
			return
		}
		if plan.StartThreadSeq == 0 || startThreadSeq < plan.StartThreadSeq {
			plan.StartThreadSeq = startThreadSeq
		}
		if endThreadSeq > plan.EndThreadSeq {
			plan.EndThreadSeq = endThreadSeq
		}
		if plan.StartContextSeq == 0 || startContextSeq < plan.StartContextSeq {
			plan.StartContextSeq = startContextSeq
		}
		if endContextSeq > plan.EndContextSeq {
			plan.EndContextSeq = endContextSeq
		}
	}

	for _, node := range nodes {
		if node.NodeID == uuid.Nil {
			continue
		}
		mergeRange(node.StartThreadSeq, node.EndThreadSeq, node.StartContextSeq, node.EndContextSeq)
		if node.Kind == FrontierNodeReplacement {
			plan.SupersededReplacementIDs = append(plan.SupersededReplacementIDs, node.NodeID)
			if node.Layer+1 > plan.Layer {
				plan.Layer = node.Layer + 1
			}
			continue
		}
		plan.SupersededChunkIDs = append(plan.SupersededChunkIDs, node.NodeID)
	}

	plan.SupersededReplacementIDs = dedupeUUIDs(plan.SupersededReplacementIDs)
	plan.SupersededChunkIDs = dedupeUUIDs(plan.SupersededChunkIDs)
	if plan.StartThreadSeq <= 0 || plan.EndThreadSeq <= 0 || plan.StartThreadSeq > plan.EndThreadSeq {
		return persistReplacementPlan{}, false, nil
	}
	if plan.StartContextSeq <= 0 || plan.EndContextSeq <= 0 || plan.StartContextSeq > plan.EndContextSeq {
		return persistReplacementPlan{}, false, fmt.Errorf("invalid context seq range for replacement plan")
	}
	return plan, true, nil
}

func writeReplacementSupersessionEdges(
	ctx context.Context,
	tx pgx.Tx,
	accountID uuid.UUID,
	threadID uuid.UUID,
	replacementID uuid.UUID,
	plan persistReplacementPlan,
) error {
	edgesRepo := data.ThreadContextSupersessionEdgesRepository{}
	for _, supersededReplacementID := range dedupeUUIDs(plan.SupersededReplacementIDs) {
		id := supersededReplacementID
		if _, err := edgesRepo.Insert(ctx, tx, data.ThreadContextSupersessionEdgeInsertInput{
			AccountID:               accountID,
			ThreadID:                threadID,
			ReplacementID:           replacementID,
			SupersededReplacementID: &id,
		}); err != nil {
			return err
		}
	}
	for _, supersededChunkID := range dedupeUUIDs(plan.SupersededChunkIDs) {
		id := supersededChunkID
		if _, err := edgesRepo.Insert(ctx, tx, data.ThreadContextSupersessionEdgeInsertInput{
			AccountID:         accountID,
			ThreadID:          threadID,
			ReplacementID:     replacementID,
			SupersededChunkID: &id,
		}); err != nil {
			return err
		}
	}
	return nil
}

func dedupeUUIDs(ids []uuid.UUID) []uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func compactReplacementLayer(nodes []FrontierNode) int {
	layer := 1
	for _, node := range nodes {
		if node.Kind == FrontierNodeReplacement && node.Layer+1 > layer {
			layer = node.Layer + 1
		}
	}
	return layer
}

func materializeCompactedPrefixFrontier(
	frontier []FrontierNode,
	compactedNodes []FrontierNode,
	summary string,
	placeholderReplacementID uuid.UUID,
) []FrontierNode {
	if len(frontier) == 0 || len(compactedNodes) == 0 || strings.TrimSpace(summary) == "" {
		return frontier
	}
	first := compactedNodes[0]
	last := compactedNodes[len(compactedNodes)-1]
	endIndex := -1
	for i, node := range frontier {
		if node.Kind != last.Kind {
			continue
		}
		if node.NodeID != last.NodeID {
			continue
		}
		if node.StartContextSeq != last.StartContextSeq || node.EndContextSeq != last.EndContextSeq {
			continue
		}
		if node.StartThreadSeq != last.StartThreadSeq || node.EndThreadSeq != last.EndThreadSeq {
			continue
		}
		endIndex = i
		break
	}
	if endIndex < 0 {
		return frontier
	}
	replacement := FrontierNode{
		Kind:            FrontierNodeReplacement,
		NodeID:          placeholderReplacementID,
		Layer:           compactReplacementLayer(compactedNodes),
		StartContextSeq: first.StartContextSeq,
		EndContextSeq:   last.EndContextSeq,
		StartThreadSeq:  first.StartThreadSeq,
		EndThreadSeq:    last.EndThreadSeq,
		SourceText:      strings.TrimSpace(summary),
		ApproxTokens:    approxTokensFromText(summary),
		Role:            "user",
	}
	out := make([]FrontierNode, 0, len(frontier)-endIndex)
	out = append(out, replacement)
	out = append(out, frontier[endIndex+1:]...)
	return out
}

func remapPersistReplacementPlan(plan persistReplacementPlan, inserted map[uuid.UUID]uuid.UUID) persistReplacementPlan {
	if len(plan.SupersededReplacementIDs) == 0 || len(inserted) == 0 {
		return plan
	}
	mapped := make([]uuid.UUID, 0, len(plan.SupersededReplacementIDs))
	for _, replacementID := range plan.SupersededReplacementIDs {
		if actualID, ok := inserted[replacementID]; ok {
			replacementID = actualID
		}
		mapped = append(mapped, replacementID)
	}
	plan.SupersededReplacementIDs = dedupeUUIDs(mapped)
	return plan
}

func needsAdditionalPreviousSummary(prefix []llm.Message, previousSummary string) bool {
	previousSummary = strings.TrimSpace(previousSummary)
	if previousSummary == "" {
		return false
	}
	leadingSummaries := compactLeadingReplacementSummaries(prefix)
	return strings.TrimSpace(strings.Join(leadingSummaries, "\n\n")) != previousSummary
}

func leadingCompactPrefixMessageCount(msgs []llm.Message, ids []uuid.UUID) int {
	if len(msgs) == 0 || len(ids) != len(msgs) {
		return 0
	}
	count := 0
	for i := range msgs {
		if ids[i] != uuid.Nil {
			break
		}
		if msgs[i].Role != "user" || len(msgs[i].Content) == 0 {
			break
		}
		count++
	}
	return count
}

func firstCompactSummaryText(msgs []llm.Message, ids []uuid.UUID) string {
	count := leadingCompactPrefixMessageCount(msgs, ids)
	if count == 0 || len(msgs[0].Content) == 0 {
		return ""
	}
	return strings.TrimSpace(msgs[0].Content[0].Text)
}

func leadingCompactSnapshotPrefixCount(msgs []llm.Message, ids []uuid.UUID) int {
	return leadingCompactPrefixMessageCount(msgs, ids)
}

// compactPrefixMessagesStillAvailable 事务内校验：待折叠的前缀消息仍全部存在，避免并发 persist 重复写 replacement。
func compactPrefixMessagesStillAvailable(ctx context.Context, tx pgx.Tx, accountID, threadID uuid.UUID, prefixIDs []uuid.UUID) (bool, error) {
	if len(prefixIDs) == 0 {
		return true, nil
	}
	var n int
	err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM messages WHERE account_id = $1 AND thread_id = $2 AND id = ANY($3::uuid[]) AND deleted_at IS NULL`,
		accountID, threadID, prefixIDs,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == len(prefixIDs), nil
}

func resolveCompactionGateway(
	ctx context.Context,
	pool CompactPersistDB,
	rc *RunContext,
	auxGateway llm.Gateway,
	emitDebugEvents bool,
	configLoader *routing.ConfigLoader,
) (llm.Gateway, string) {
	fallbackGateway := rc.Gateway
	fallbackModel := ""
	if rc.SelectedRoute != nil {
		fallbackModel = rc.SelectedRoute.Route.Model
	}

	var selector string
	err := pool.QueryRow(ctx,
		`SELECT value FROM platform_settings WHERE key = $1`,
		settingContextCompactionModel,
	).Scan(&selector)
	selector = strings.TrimSpace(selector)
	if err != nil || selector == "" {
		return fallbackGateway, fallbackModel
	}
	if configLoader == nil {
		return fallbackGateway, fallbackModel
	}
	aid := rc.Run.AccountID
	routingCfg, err := configLoader.Load(ctx, &aid)
	if err != nil {
		slog.WarnContext(ctx, "context_compact", "phase", "routing_load", "err", err.Error())
		return fallbackGateway, fallbackModel
	}
	selected, err := resolveSelectedRouteBySelector(routingCfg, selector, map[string]any{}, rc.RoutingByokEnabled)
	if err != nil || selected == nil {
		if err != nil {
			slog.WarnContext(ctx, "context_compact", "phase", "selector", "selector", selector, "err", err.Error())
		}
		return fallbackGateway, fallbackModel
	}
	gw, err := gatewayFromSelectedRoute(*selected, auxGateway, emitDebugEvents, rc.LlmMaxResponseBytes)
	if err != nil {
		slog.WarnContext(ctx, "context_compact", "phase", "gateway_build", "err", err.Error())
		return fallbackGateway, fallbackModel
	}
	return gw, selected.Route.Model
}

func compactPersistTriggerTokens(cfg ContextCompactSettings, windowFromRoute int) (trigger int, window int) {
	window = windowFromRoute
	if window <= 0 {
		window = cfg.FallbackContextWindowTokens
	}
	pct := cfg.PersistTriggerContextPct
	if pct > 100 {
		pct = 100
	}
	if pct > 0 && window > 0 {
		trigger = window * pct / 100
		if trigger < 1 {
			trigger = 1
		}
		return trigger, window
	}
	trigger = cfg.PersistTriggerApproxTokens
	return trigger, window
}

func inlineCompactEstimatePressure(
	rc *RunContext,
	msgs []llm.Message,
	anchor *ContextCompactPressureAnchor,
) (int, ContextCompactPressureStats) {
	estimate := HistoryThreadPromptTokensForRoute(rc.SelectedRoute, msgs)
	return estimate, ComputeContextCompactPressure(estimate, anchor)
}

func MaybeInlineCompactMessages(
	ctx context.Context,
	rc *RunContext,
	msgs []llm.Message,
	anchor *ContextCompactPressureAnchor,
	forceCompact bool,
) ([]llm.Message, ContextCompactPressureStats, bool, error) {
	if rc == nil {
		return msgs, ContextCompactPressureStats{}, false, nil
	}
	cfg := rc.ContextCompact
	if !cfg.PersistEnabled || rc.Gateway == nil || rc.SelectedRoute == nil {
		estimate := HistoryThreadPromptTokensForRoute(rc.SelectedRoute, msgs)
		stats := ComputeContextCompactPressure(estimate, anchor)
		return msgs, stats, false, nil
	}
	enc, err := ResolveTiktokenForRoute(rc.SelectedRoute)
	if err != nil || enc == nil {
		enc, _ = tiktoken.GetEncoding(tiktoken.MODEL_O200K_BASE)
	}
	window := routing.RouteContextWindowTokens(rc.SelectedRoute.Route)
	trigger, window := compactPersistTriggerTokens(cfg, window)
	working := append([]llm.Message(nil), msgs...)
	estimate := HistoryThreadPromptTokens(enc, working)
	stats := ComputeContextCompactPressure(estimate, anchor)
	if !forceCompact && stats.ContextPressureTokens < trigger {
		return msgs, stats, false, nil
	}
	if len(working) == 1 {
		compactedSingle, changed, compactErr := maybeInlineCompactSingleOversizedTextAtom(ctx, rc, working[0], enc)
		if compactErr != nil {
			return msgs, stats, false, compactErr
		}
		if changed {
			stats.TargetChunkCount = len(buildCanonicalCompactChunks(enc, working))
			stats.SingleAtomPartial = true
			return compactedSingle, stats, true, nil
		}
		return msgs, stats, false, nil
	}

	targetTokens := contextCompactTargetTokens(cfg, window)
	if targetTokens <= 0 {
		targetTokens = trigger
	}
	if targetTokens >= stats.ContextPressureTokens {
		if forceCompact {
			targetTokens = stats.ContextPressureTokens - 1
		} else {
			targetTokens = trigger - 1
		}
	}
	if targetTokens < 1 {
		targetTokens = 1
	}

	changedAny := false
	compactDeadline := time.Now().Add(contextCompactTimeBudget)
	inlineRound := 0
	forceRound := forceCompact
	for forceRound || stats.ContextPressureTokens > targetTokens {
		if time.Now().After(compactDeadline) {
			slog.WarnContext(ctx, "context_compact", "phase", "inline_time_budget_exceeded", "run_id", rc.Run.ID.String(), "round", inlineRound)
			break
		}
		inlineRound++
		forceRound = false
		roundStartEvent := map[string]any{
			"op":              "inline",
			"phase":           "round_started",
			"round":           inlineRound,
			"atoms_selected":  0,
			"pressure_tokens": stats.ContextPressureTokens,
			"target_tokens":   targetTokens,
		}
		if err := appendContextCompactRunEvent(ctx, nil, nil, rc, roundStartEvent); err != nil {
			slog.WarnContext(ctx, "context_compact", "phase", "inline_round_started_event", "err", err.Error(), "run_id", rc.Run.ID.String())
		}
		slog.InfoContext(ctx, "context_compact",
			"op", "inline",
			"phase", "round_started",
			"run_id", rc.Run.ID.String(),
			"round", inlineRound,
		)
		beforeEstimate := estimate
		nodes := buildCompactFrontierAtomsFromMessagesWithOptions(enc, working, false)
		selection := selectCompactAtomWindow(nodes, stats.ContextPressureTokens-targetTokens, contextCompactMaxLLMInputTokens)
		if len(selection.Nodes) == 0 {
			break
		}
		progress := newCompactProgressRecorder(nil, nil, map[string]any{
			"op":    "inline",
			"mode":  "canonical_atoms",
			"round": inlineRound,
		})
		summary, usedNodes, compactErr := compactNodesWithShrinkRetry(ctx, rc, rc.Gateway, rc.SelectedRoute.Route.Model, selection.Nodes, progress)
		if compactErr != nil {
			return msgs, stats, changedAny, compactErr
		}
		summary = strings.TrimSpace(summary)
		if summary == "" || len(usedNodes) == 0 {
			break
		}
		working = materializeCompactedPrefixAtoms(working, nodes, len(usedNodes)-1, summary)
		working = truncateLargeTailMessages(enc, working)
		stats.TargetChunkCount = len(usedNodes)
		stats.PreviousReplacementCount = 0
		stats.SingleAtomPartial = false
		estimate = HistoryThreadPromptTokens(enc, working)
		stats = ComputeContextCompactPressure(estimate, anchor)
		changedAny = true
		forceRound = false
		roundCompletedEvent := map[string]any{
			"op":              "inline",
			"phase":           "round_completed",
			"round":           inlineRound,
			"atoms_compacted": len(usedNodes),
			"pressure_tokens": stats.ContextPressureTokens,
			"target_tokens":   targetTokens,
		}
		if err := appendContextCompactRunEvent(ctx, nil, nil, rc, roundCompletedEvent); err != nil {
			slog.WarnContext(ctx, "context_compact", "phase", "inline_round_completed_event", "err", err.Error(), "run_id", rc.Run.ID.String())
		}
		systemPrompt := compactSystemPromptForRun(ctx, rc, contextCompactSystemPrompt, nil)
		notifyCompactApplied(ctx, rc, CompactInput{
			SystemPrompt: systemPrompt,
			Messages:     append([]llm.Message(nil), msgs...),
		}, CompactOutput{
			SystemPrompt: systemPrompt,
			Messages:     append([]llm.Message(nil), working...),
			Summary:      summary,
			Changed:      true,
		})
		if estimate >= beforeEstimate {
			break
		}
		if cfg.PersistTriggerContextPct <= 0 {
			break
		}
	}
	return working, stats, changedAny, nil
}

func maybeInlineCompactSingleOversizedTextAtom(
	ctx context.Context,
	rc *RunContext,
	msg llm.Message,
	enc *tiktoken.Tiktoken,
) ([]llm.Message, bool, error) {
	role := strings.TrimSpace(msg.Role)
	if role != "user" && role != "assistant" {
		return nil, false, nil
	}
	if role == "assistant" && len(msg.ToolCalls) > 0 {
		return nil, false, nil
	}
	text := strings.TrimSpace(messageText(msg))
	if text == "" {
		text = compactFallbackContentText(msg)
	}
	if text == "" {
		return nil, false, nil
	}
	pieces := splitCompactPayload(enc, text)
	if len(pieces) < 2 {
		return nil, false, nil
	}
	keepTail := len(pieces) * 30 / 100
	if keepTail < 1 {
		keepTail = 1
	}
	headParts := pieces[:len(pieces)-keepTail]
	tailParts := pieces[len(pieces)-keepTail:]
	if len(headParts) == 0 || len(tailParts) == 0 {
		return nil, false, nil
	}
	summary, err := runContextCompactLLM(ctx, rc, rc.Gateway, rc.SelectedRoute.Route.Model, []llm.Message{{
		Role:    role,
		Content: []llm.TextPart{{Text: strings.Join(headParts, "\n\n")}},
	}}, enc, "")
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(summary) == "" {
		return nil, false, nil
	}
	tailMsg := llm.Message{
		Role:    role,
		Phase:   msg.Phase,
		Content: []llm.TextPart{{Text: strings.TrimSpace(strings.Join(tailParts, "\n\n"))}},
	}
	out := []llm.Message{makeThreadContextReplacementMessage(summary), tailMsg}
	systemPrompt := compactSystemPromptForRun(ctx, rc, contextCompactSystemPrompt, []llm.Message{msg})
	notifyCompactApplied(ctx, rc, CompactInput{
		SystemPrompt: systemPrompt,
		Messages:     []llm.Message{msg},
	}, CompactOutput{
		SystemPrompt: systemPrompt,
		Messages:     append([]llm.Message(nil), out...),
		Summary:      strings.TrimSpace(summary),
		Changed:      true,
	})
	return out, true, nil
}

func runContextCompactLLM(ctx context.Context, rc *RunContext, gateway llm.Gateway, model string, prefix []llm.Message, enc *tiktoken.Tiktoken, previousSummary string) (string, error) {
	_ = previousSummary
	return runContextCompactLLMForNodes(ctx, rc, gateway, model, buildCompactFrontierNodesFromMessages(enc, prefix), compactProgressRecorder{}, 1)
}

func trimLeadingCompactSnapshotMessages(msgs []llm.Message) []llm.Message {
	return msgs
}

func appendContextCompactRunEvent(
	ctx context.Context,
	pool CompactPersistDB,
	eventsRepo CompactRunEventAppender,
	rc *RunContext,
	data map[string]any,
) error {
	ev := rc.Emitter.Emit("run.context_compact", data, nil, nil)
	if eventsRepo == nil || pool == nil {
		notifyRunEventSubscribers(ctx, rc)
		return nil
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err := eventsRepo.AppendRunEvent(ctx, tx, rc.Run.ID, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	notifyRunEventSubscribers(ctx, rc)
	return nil
}

func emitContextCompactFailure(
	ctx context.Context,
	postCtx context.Context,
	pool CompactPersistDB,
	eventsRepo CompactRunEventAppender,
	rc *RunContext,
	op string,
	phase string,
	err error,
) {
	if err == nil {
		return
	}
	payload := map[string]any{
		"op":    op,
		"phase": phase,
		"error": err.Error(),
	}
	if appendErr := appendContextCompactRunEvent(postCtx, pool, eventsRepo, rc, payload); appendErr != nil {
		slog.WarnContext(ctx, "context_compact", "phase", "run_event_failure", "err", appendErr.Error(), "run_id", rc.Run.ID.String())
	}
}

// serializeMessagesForCompact 将消息列表序列化为摘要 LLM 可读的纯文本。
// active snapshot 通过 previousSummary 单独传递；这里仅处理真实对话与 replay 内容。
// tool result 只提取 result/error 核心内容，tool calls 展开参数，避免噪声。
func serializeMessagesForCompact(msgs []llm.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if text := strings.TrimSpace(messageText(m)); text != "" {
				parts = append(parts, "[User]: "+text)
			}
		case "assistant":
			if text := strings.TrimSpace(messageText(m)); text != "" {
				parts = append(parts, "[Assistant]: "+text)
			}
			if len(m.ToolCalls) > 0 {
				calls := make([]string, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					call := tc.ToolName
					if len(tc.ArgumentsJSON) > 0 {
						if args, err := json.Marshal(tc.ArgumentsJSON); err == nil {
							call += "(" + string(args) + ")"
						}
					}
					calls = append(calls, call)
				}
				parts = append(parts, "[Assistant tool calls]: "+strings.Join(calls, "; "))
			}
		case "tool":
			// tool result Content 是 JSON envelope {tool_call_id, tool_name, result?, error?}
			// 只取 tool_name + result/error，丢弃 tool_call_id 等无关字段
			if text := strings.TrimSpace(messageText(m)); text != "" {
				label := "[Tool result]"
				content := text
				var envelope map[string]any
				if err := json.Unmarshal([]byte(text), &envelope); err == nil {
					if name, _ := envelope["tool_name"].(string); name != "" {
						label = "[Tool result: " + name + "]"
					}
					// 优先取 error，其次取 result
					if errVal := envelope["error"]; errVal != nil {
						if b, err := json.Marshal(errVal); err == nil {
							content = string(b)
						}
					} else if resVal := envelope["result"]; resVal != nil {
						if b, err := json.Marshal(resVal); err == nil {
							content = string(b)
						}
					}
				}
				parts = append(parts, label+": "+content)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// runGroupCompactLLM 群聊专用的 compact LLM 调用，使用群聊 prompt 模板。
func runGroupCompactLLM(ctx context.Context, gateway llm.Gateway, model string, prefix []llm.Message, enc *tiktoken.Tiktoken, previousSummary string) (string, error) {
	return runContextCompactLLM(ctx, nil, gateway, model, prefix, enc, previousSummary)
}
