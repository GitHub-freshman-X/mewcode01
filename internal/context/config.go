package context

import "fmt"

type Config struct {
	WindowTokens              int
	SummaryOutputTokens       int
	AutoSafetyTokens          int
	ManualSafetyTokens        int
	SingleResultChars         int
	MessageResultChars        int
	PreviewChars              int
	RecentTokens              int
	RecentMessageMinimum      int
	DebugSummaryResponseChars int
}

func DefaultConfig() Config {
	return Config{WindowTokens: 200000, SummaryOutputTokens: 20000, AutoSafetyTokens: 13000, ManualSafetyTokens: 3000, SingleResultChars: 50000, MessageResultChars: 200000, PreviewChars: 2000, RecentTokens: 10000, RecentMessageMinimum: 5}
}

func (c Config) Validate() error {
	if c.WindowTokens <= c.SummaryOutputTokens+c.AutoSafetyTokens || c.WindowTokens <= c.SummaryOutputTokens+c.ManualSafetyTokens {
		return fmt.Errorf("context window is too small")
	}
	if c.SingleResultChars <= 0 || c.MessageResultChars <= 0 || c.PreviewChars <= 0 || c.RecentTokens <= 0 || c.RecentMessageMinimum <= 0 {
		return fmt.Errorf("context budgets must be positive")
	}
	return nil
}

type Trigger string

const (
	TriggerAutomatic Trigger = "automatic"
	TriggerManual    Trigger = "manual"
	TriggerForced    Trigger = "forced"
	TriggerEmergency Trigger = "emergency"
)
