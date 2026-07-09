package provider

type PromptBundle struct {
	StableSystem  string
	DynamicSystem []SystemMessage
	CachePolicy   CachePolicy
}

type SystemMessage struct {
	Tag       string
	Content   string
	Cacheable bool
}

type CachePolicy struct {
	Enable          bool
	StableSystem    bool
	StableTools     bool
	DynamicMessages bool
}
