package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestApplyAgentOverridesClampsMaxCompletionTokens(t *testing.T) {
	agent := &types.CustomAgent{
		Config: types.CustomAgentConfig{
			MaxCompletionTokens: maxAgentCompletionTokens + 1,
		},
	}
	cm := &types.ChatManage{}

	(&sessionService{}).applyAgentOverridesToChatManage(context.Background(), agent, cm)

	if got := cm.SummaryConfig.MaxCompletionTokens; got != maxAgentCompletionTokens {
		t.Fatalf("MaxCompletionTokens = %d, want %d", got, maxAgentCompletionTokens)
	}
}

func TestApplyAgentOverridesKeepsSafeMaxCompletionTokens(t *testing.T) {
	agent := &types.CustomAgent{
		Config: types.CustomAgentConfig{
			MaxCompletionTokens: 4096,
		},
	}
	cm := &types.ChatManage{}

	(&sessionService{}).applyAgentOverridesToChatManage(context.Background(), agent, cm)

	if got := cm.SummaryConfig.MaxCompletionTokens; got != 4096 {
		t.Fatalf("MaxCompletionTokens = %d, want 4096", got)
	}
}

func TestApplyAgentOverridesDisablesThinkingForBuiltinQuickAnswer(t *testing.T) {
	enabled := true
	agent := &types.CustomAgent{
		ID: types.BuiltinQuickAnswerID,
		Config: types.CustomAgentConfig{
			AgentMode: types.AgentModeQuickAnswer,
			Thinking:  &enabled,
		},
	}
	cm := &types.ChatManage{}

	(&sessionService{}).applyAgentOverridesToChatManage(context.Background(), agent, cm)

	if cm.SummaryConfig.Thinking == nil {
		t.Fatal("Thinking = nil, want false")
	}
	if *cm.SummaryConfig.Thinking {
		t.Fatal("Thinking = true, want false")
	}
}
