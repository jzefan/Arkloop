package risk

// Level 表示风险等级。
type Level string

const (
	LevelLow      Level = "low"
	LevelMedium   Level = "medium"
	LevelHigh     Level = "high"
	LevelCritical Level = "critical"
)

// Score 是一次风险评估的结果。
type Score struct {
	Value   int    // 0-100
	Level   Level
	Signals []string // 触发的风险信号描述
}

func scoreToLevel(v int) Level {
	switch {
	case v >= 80:
		return LevelCritical
	case v >= 50:
		return LevelHigh
	case v >= 25:
		return LevelMedium
	default:
		return LevelLow
	}
}
