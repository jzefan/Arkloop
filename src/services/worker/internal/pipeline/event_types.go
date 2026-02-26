package pipeline

// Human-in-the-loop 事件类型常量，供 executor 和 mw_cancel_guard 共用。
const (
	EventTypeInputRequested = "run.input_requested"
	EventTypeInputProvided  = "run.input_provided"
)
