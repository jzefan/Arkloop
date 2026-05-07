package data

import (
	"arkloop/services/shared/scheduledjobs"
)

// ScheduledJob 是 scheduled_jobs 表的行。
type ScheduledJob = scheduledjobs.ScheduledJob

// ScheduledJobWithTrigger 附带 trigger 的 next_fire_at。
type ScheduledJobWithTrigger = scheduledjobs.ScheduledJobWithTrigger

// UpdateJobParams 是 UpdateJob 的部分更新参数。
type UpdateJobParams = scheduledjobs.UpdateJobParams
