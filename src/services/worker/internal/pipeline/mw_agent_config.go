package pipeline

import "context"

func NewAgentConfigMiddleware(_ any) RunMiddleware {
	return func(ctx context.Context, rc *RunContext, next RunHandler) error {
		return next(ctx, rc)
	}
}
