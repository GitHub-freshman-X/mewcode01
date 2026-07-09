package prompt

type Mode string

const (
	ModeAct  Mode = "act"
	ModePlan Mode = "plan"
	ModeDo   Mode = "do"
)

type InjectionKind string

const (
	InjectionFull     InjectionKind = "full"
	InjectionReminder InjectionKind = "reminder"
	InjectionBrief    InjectionKind = "brief"
	InjectionNone     InjectionKind = "none"
)

type ModeInjection struct {
	Kind      InjectionKind
	Content   string
	Cacheable bool
}

type InjectionPolicy struct {
	ReminderEvery int
	BriefEvery    int
}

func DefaultInjectionPolicy() InjectionPolicy {
	return InjectionPolicy{ReminderEvery: 3, BriefEvery: 1}
}

func NormalizeInjectionPolicy(p InjectionPolicy) InjectionPolicy {
	if p.ReminderEvery < 0 {
		p.ReminderEvery = 0
	}
	if p.BriefEvery < 0 {
		p.BriefEvery = 0
	}
	if p.ReminderEvery == 0 && p.BriefEvery == 0 {
		return DefaultInjectionPolicy()
	}
	return p
}

func ModeInjectionFor(mode Mode, iteration int, policy InjectionPolicy) ModeInjection {
	policy = NormalizeInjectionPolicy(policy)
	if iteration <= 1 {
		return ModeInjection{Kind: InjectionFull, Content: fullModeText(mode)}
	}
	if policy.ReminderEvery > 0 && iteration%policy.ReminderEvery == 0 {
		return ModeInjection{Kind: InjectionReminder, Content: reminderModeText(mode)}
	}
	if policy.BriefEvery > 0 && iteration%policy.BriefEvery == 0 {
		return ModeInjection{Kind: InjectionBrief, Content: briefModeText(mode)}
	}
	return ModeInjection{Kind: InjectionNone}
}

func fullModeText(mode Mode) string {
	switch mode {
	case ModePlan:
		return "当前是规划模式。只能进行只读探索，不得修改文件、运行有副作用的命令或请求副作用工具。最终输出一份可执行计划。"
	case ModeDo:
		return "当前是执行计划模式。按用户消息中的待执行计划顺序执行，必要时读取现状并保持计划意图；使用当前可用工具完成验证和落地。"
	default:
		return "当前是普通执行模式。可以在可用工具范围内读取、修改和验证，所有修改都应基于已读取的上下文。"
	}
}

func reminderModeText(mode Mode) string {
	switch mode {
	case ModePlan:
		return "规划模式提醒：保持只读探索，不得修改文件或请求副作用工具，最终给出计划。"
	case ModeDo:
		return "执行模式提醒：继续按待执行计划推进，保留所有计划意图。"
	default:
		return "执行提醒：按当前任务推进，修改前先读取，修改后验证。"
	}
}

func briefModeText(mode Mode) string {
	switch mode {
	case ModePlan:
		return "规划模式：只读探索并输出计划。"
	case ModeDo:
		return "执行计划模式：按计划推进。"
	default:
		return "普通执行模式。"
	}
}
