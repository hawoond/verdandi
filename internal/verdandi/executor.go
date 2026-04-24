package verdandi

type Executor struct {
	tool Tool
}

func NewExecutor(options Options) Executor {
	return Executor{tool: NewTool(options)}
}

func (e Executor) Execute(name string, args map[string]any) (map[string]any, error) {
	return e.tool.Handle(name, args)
}
