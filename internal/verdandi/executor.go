package verdandi

import "context"

type Executor struct {
	tool Tool
}

func NewExecutor(options Options) Executor {
	return Executor{tool: NewTool(options)}
}

func (e Executor) Execute(name string, args map[string]any) (map[string]any, error) {
	return e.tool.Handle(name, args)
}

func (e Executor) ExecuteWithProgress(name string, args map[string]any, reporter ProgressReporter) (map[string]any, error) {
	return e.tool.HandleWithProgress(name, args, reporter)
}

func (e Executor) ExecuteWithContext(ctx context.Context, name string, args map[string]any, reporter ProgressReporter) (map[string]any, error) {
	return e.tool.HandleContext(ctx, name, args, reporter)
}
