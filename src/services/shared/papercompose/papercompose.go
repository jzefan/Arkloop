// Package papercompose is a storage-agnostic, deterministic paper composer.
//
// It is the single source of truth for selecting a subset of a question pool
// that satisfies type / difficulty / knowledge-point distribution constraints.
// Both composition paths import it:
//
//   - kb (book-tutor "智能组卷")  → pool comes from the local 组卷题库
//   - exam (命题专家 build_paper)  → pool comes from the exam question bank
//
// Compose is a pure function: it never mutates the caller's slice and, given the
// same (pool, spec, seed), always returns the same selection.
package papercompose

import (
	"sort"
	"strings"
)

// Question is the minimal view of a question needed for composition. Callers map
// their own richer rows to this shape and back via the stable ID.
type Question struct {
	ID               string
	Type             string
	Difficulty       string
	KnowledgePointID string
}

// Spec describes the composition constraints. Total is required; the three
// distribution maps are optional (nil = no constraint on that dimension).
type Spec struct {
	Total          int
	TypeDist       map[string]int
	DifficultyDist map[string]int
	KPDist         map[string]int
}

// Shortage is a structured warning explaining why the pool cannot satisfy Spec.
//
//	Dimension: "type" | "difficulty" | "knowledge_point_id" | "" (total pool)
//	Value:     the specific value short of supply, "all" (over-constrained), or ""
type Shortage struct {
	Dimension string
	Value     string
	Requested int
	Available int
	Message   string
}

const (
	dimType       = "type"
	dimDifficulty = "difficulty"
	dimKP         = "knowledge_point_id"

	msgTypeShortage = "题型题量不足"
	msgDiffShortage = "难度题量不足"
	msgKPShortage   = "知识点题量不足"
	msgOverTotal    = "约束题量超过试卷总题数"
	msgPoolShortage = "题池题量不足"
	msgUnmet        = "约束无法同时满足"
)

// ShortagesToMaps renders typed shortages into the legacy warning shape consumed
// by both composition paths' tool results and UI panels:
//
//	{ <dimension>: <value>, "available": N, "requested": M, "message": "..." }
//
// For a total-pool shortage (Dimension == "") the dimension key is omitted.
func ShortagesToMaps(shortages []Shortage) []map[string]any {
	out := make([]map[string]any, 0, len(shortages))
	for _, s := range shortages {
		m := map[string]any{
			"available": s.Available,
			"requested": s.Requested,
			"message":   s.Message,
		}
		if s.Dimension != "" {
			m[s.Dimension] = s.Value
		}
		out = append(out, m)
	}
	return out
}

// Compose selects Spec.Total questions from pool honouring the distribution
// constraints. On success it returns the ordered selection and a nil/empty
// shortage slice. When the pool is insufficient it returns nil and one or more
// Shortage entries describing the gap. The caller's pool is never mutated.
func Compose(pool []Question, spec Spec, seed int64) ([]Question, []Shortage) {
	// Work on a copy so callers never observe reordering.
	work := make([]Question, len(pool))
	copy(work, pool)

	// Deterministic baseline order, then optional seeded shuffle.
	sort.Slice(work, func(i, j int) bool { return work[i].ID < work[j].ID })
	if seed != 0 {
		shuffleSeeded(work, seed)
	}

	if sh := validateDistributions(spec.Total, []dimensionSpec{
		{dim: dimType, msg: msgTypeShortage, requested: spec.TypeDist, available: countBy(work, func(q Question) string { return q.Type })},
		{dim: dimDifficulty, msg: msgDiffShortage, requested: spec.DifficultyDist, available: countBy(work, func(q Question) string { return q.Difficulty })},
		{dim: dimKP, msg: msgKPShortage, requested: spec.KPDist, available: countBy(work, func(q Question) string { return q.KnowledgePointID })},
	}); len(sh) > 0 {
		return nil, sh
	}

	if len(work) < spec.Total {
		return nil, []Shortage{{Requested: spec.Total, Available: len(work), Message: msgPoolShortage}}
	}

	selected := greedySelect(work, spec)

	if sh := unmetWarnings(selected, spec); len(sh) > 0 {
		return nil, sh
	}
	return selected, nil
}

type dimensionSpec struct {
	dim       string
	msg       string
	requested map[string]int
	available map[string]int
}

func validateDistributions(total int, specs []dimensionSpec) []Shortage {
	var out []Shortage
	for _, s := range specs {
		if sumPositive(s.requested) > total {
			out = append(out, Shortage{
				Dimension: s.dim,
				Value:     "all",
				Requested: sumPositive(s.requested),
				Available: total,
				Message:   msgOverTotal,
			})
			continue
		}
		for value, requested := range s.requested {
			if requested <= 0 {
				continue
			}
			if avail := s.available[value]; avail < requested {
				out = append(out, Shortage{
					Dimension: s.dim,
					Value:     value,
					Requested: requested,
					Available: avail,
					Message:   s.msg,
				})
			}
		}
	}
	return out
}

// greedySelect repeatedly picks the unused question that satisfies the most
// still-unmet constraints (type + difficulty + kp), filling Total slots.
func greedySelect(pool []Question, spec Spec) []Question {
	remainingType := copyPositive(spec.TypeDist)
	remainingDiff := copyPositive(spec.DifficultyDist)
	remainingKP := copyPositive(spec.KPDist)
	selected := make([]Question, 0, spec.Total)
	used := make(map[string]bool, spec.Total)

	for len(selected) < spec.Total {
		best := -1
		bestScore := -1
		for i, q := range pool {
			if used[q.ID] {
				continue
			}
			score := unmetScore(q.Type, remainingType) +
				unmetScore(q.Difficulty, remainingDiff) +
				unmetScore(q.KnowledgePointID, remainingKP)
			if score > bestScore {
				bestScore = score
				best = i
			}
		}
		if best < 0 {
			break
		}
		q := pool[best]
		selected = append(selected, q)
		used[q.ID] = true
		decrement(remainingType, q.Type)
		decrement(remainingDiff, q.Difficulty)
		decrement(remainingKP, q.KnowledgePointID)
	}
	return selected
}

func unmetWarnings(selected []Question, spec Spec) []Shortage {
	return validateDistributions(len(selected), []dimensionSpec{
		{dim: dimType, msg: msgUnmet, requested: spec.TypeDist, available: countBy(selected, func(q Question) string { return q.Type })},
		{dim: dimDifficulty, msg: msgUnmet, requested: spec.DifficultyDist, available: countBy(selected, func(q Question) string { return q.Difficulty })},
		{dim: dimKP, msg: msgUnmet, requested: spec.KPDist, available: countBy(selected, func(q Question) string { return q.KnowledgePointID })},
	})
}

func countBy(pool []Question, attr func(Question) string) map[string]int {
	out := map[string]int{}
	for _, q := range pool {
		if v := strings.TrimSpace(attr(q)); v != "" {
			out[v]++
		}
	}
	return out
}

func sumPositive(in map[string]int) int {
	total := 0
	for _, v := range in {
		if v > 0 {
			total += v
		}
	}
	return total
}

func copyPositive(in map[string]int) map[string]int {
	out := map[string]int{}
	for k, v := range in {
		if strings.TrimSpace(k) != "" && v > 0 {
			out[k] = v
		}
	}
	return out
}

func unmetScore(value string, remaining map[string]int) int {
	if remaining[strings.TrimSpace(value)] > 0 {
		return 1
	}
	return 0
}

func decrement(remaining map[string]int, value string) {
	value = strings.TrimSpace(value)
	if remaining[value] > 0 {
		remaining[value]--
	}
}

// shuffleSeeded performs a deterministic Fisher–Yates shuffle driven by a
// 64-bit splitmix-style PRNG, so we don't depend on math/rand global state.
func shuffleSeeded(qs []Question, seed int64) {
	state := uint64(seed)
	next := func() uint64 {
		state += 0x9E3779B97F4A7C15
		z := state
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		return z ^ (z >> 31)
	}
	for i := len(qs) - 1; i > 0; i-- {
		j := int(next() % uint64(i+1))
		qs[i], qs[j] = qs[j], qs[i]
	}
}
