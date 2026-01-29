package provider

import (
	"context"
	"strings"
	"sync"

	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

// Scope represents a Slack OAuth scope
type Scope string

const (
	ScopeChannelsRead    Scope = "channels:read"
	ScopeChannelsHistory Scope = "channels:history"
	ScopeGroupsRead      Scope = "groups:read"
	ScopeGroupsHistory   Scope = "groups:history"
	ScopeIMRead          Scope = "im:read"
	ScopeIMHistory       Scope = "im:history"
	ScopeIMWrite         Scope = "im:write"
	ScopeMPIMRead        Scope = "mpim:read"
	ScopeMPIMHistory     Scope = "mpim:history"
	ScopeMPIMWrite       Scope = "mpim:write"
	ScopeUsersRead       Scope = "users:read"
	ScopeChatWrite       Scope = "chat:write"
	ScopeSearchRead      Scope = "search:read"
)

// ScopeDetector detects available scopes by testing API calls
type ScopeDetector struct {
	client SlackAPI
	logger *zap.Logger

	detected      map[Scope]bool
	detectedMutex sync.RWMutex
}

// NewScopeDetector creates a new scope detector
func NewScopeDetector(client SlackAPI, logger *zap.Logger) *ScopeDetector {
	return &ScopeDetector{
		client:   client,
		logger:   logger,
		detected: make(map[Scope]bool),
	}
}

// DetectScopes attempts to detect available scopes by making test API calls
func (sd *ScopeDetector) DetectScopes(ctx context.Context) {
	sd.logger.Info("Detecting available OAuth scopes...",
		zap.String("context", "console"),
	)

	var wg sync.WaitGroup
	scopeTests := []struct {
		scope Scope
		test  func(context.Context) bool
	}{
		{ScopeChannelsRead, sd.testChannelsRead},
		{ScopeGroupsRead, sd.testGroupsRead},
		{ScopeIMRead, sd.testIMRead},
		{ScopeMPIMRead, sd.testMPIMRead},
		{ScopeUsersRead, sd.testUsersRead},
		{ScopeSearchRead, sd.testSearchRead},
		// History scopes are harder to test without actual channels,
		// so we infer them from read scopes
	}

	for _, st := range scopeTests {
		wg.Add(1)
		go func(scope Scope, testFunc func(context.Context) bool) {
			defer wg.Done()
			available := testFunc(ctx)
			sd.setScope(scope, available)

			if available {
				sd.logger.Debug("Scope available",
					zap.String("scope", string(scope)),
				)
			} else {
				sd.logger.Warn("Scope not available",
					zap.String("scope", string(scope)),
					zap.String("context", "console"),
				)
			}
		}(st.scope, st.test)
	}

	wg.Wait()

	// Infer history scopes from read scopes (if you can read, you likely can read history)
	sd.inferHistoryScopes()

	sd.logDetectedScopes()
}

func (sd *ScopeDetector) setScope(scope Scope, available bool) {
	sd.detectedMutex.Lock()
	defer sd.detectedMutex.Unlock()
	sd.detected[scope] = available
}

func (sd *ScopeDetector) inferHistoryScopes() {
	sd.detectedMutex.Lock()
	defer sd.detectedMutex.Unlock()

	// If channels:read is available, assume channels:history is too
	// This is a reasonable assumption since they're typically granted together
	if sd.detected[ScopeChannelsRead] {
		sd.detected[ScopeChannelsHistory] = true
	}
	if sd.detected[ScopeGroupsRead] {
		sd.detected[ScopeGroupsHistory] = true
	}
	if sd.detected[ScopeIMRead] {
		sd.detected[ScopeIMHistory] = true
	}
	if sd.detected[ScopeMPIMRead] {
		sd.detected[ScopeMPIMHistory] = true
	}
}

func (sd *ScopeDetector) logDetectedScopes() {
	sd.detectedMutex.RLock()
	defer sd.detectedMutex.RUnlock()

	var available []string
	var unavailable []string

	for scope, isAvailable := range sd.detected {
		if isAvailable {
			available = append(available, string(scope))
		} else {
			unavailable = append(unavailable, string(scope))
		}
	}

	sd.logger.Info("Scope detection complete",
		zap.String("context", "console"),
		zap.Strings("available", available),
		zap.Strings("unavailable", unavailable),
	)
}

// HasScope checks if a scope is available
func (sd *ScopeDetector) HasScope(scope Scope) bool {
	sd.detectedMutex.RLock()
	defer sd.detectedMutex.RUnlock()
	return sd.detected[scope]
}

// HasAnyReadScope checks if any read scope is available for channels
func (sd *ScopeDetector) HasAnyReadScope() bool {
	return sd.HasScope(ScopeChannelsRead) ||
		sd.HasScope(ScopeGroupsRead) ||
		sd.HasScope(ScopeIMRead) ||
		sd.HasScope(ScopeMPIMRead)
}

// HasAnyHistoryScope checks if any history scope is available
func (sd *ScopeDetector) HasAnyHistoryScope() bool {
	return sd.HasScope(ScopeChannelsHistory) ||
		sd.HasScope(ScopeGroupsHistory) ||
		sd.HasScope(ScopeIMHistory) ||
		sd.HasScope(ScopeMPIMHistory)
}

// AvailableChannelTypes returns the channel types that can be accessed
func (sd *ScopeDetector) AvailableChannelTypes() []string {
	var types []string
	if sd.HasScope(ScopeChannelsRead) {
		types = append(types, "public_channel")
	}
	if sd.HasScope(ScopeGroupsRead) {
		types = append(types, "private_channel")
	}
	if sd.HasScope(ScopeIMRead) {
		types = append(types, "im")
	}
	if sd.HasScope(ScopeMPIMRead) {
		types = append(types, "mpim")
	}
	return types
}

// Test functions for each scope

func (sd *ScopeDetector) testChannelsRead(ctx context.Context) bool {
	_, _, err := sd.client.GetConversationsContext(ctx, &slack.GetConversationsParameters{
		Types: []string{"public_channel"},
		Limit: 1,
	})
	return !isMissingScopeError(err)
}

func (sd *ScopeDetector) testGroupsRead(ctx context.Context) bool {
	_, _, err := sd.client.GetConversationsContext(ctx, &slack.GetConversationsParameters{
		Types: []string{"private_channel"},
		Limit: 1,
	})
	return !isMissingScopeError(err)
}

func (sd *ScopeDetector) testIMRead(ctx context.Context) bool {
	_, _, err := sd.client.GetConversationsContext(ctx, &slack.GetConversationsParameters{
		Types: []string{"im"},
		Limit: 1,
	})
	return !isMissingScopeError(err)
}

func (sd *ScopeDetector) testMPIMRead(ctx context.Context) bool {
	_, _, err := sd.client.GetConversationsContext(ctx, &slack.GetConversationsParameters{
		Types: []string{"mpim"},
		Limit: 1,
	})
	return !isMissingScopeError(err)
}

func (sd *ScopeDetector) testUsersRead(ctx context.Context) bool {
	_, err := sd.client.GetUsersContext(ctx, slack.GetUsersOptionLimit(1))
	return !isMissingScopeError(err)
}

func (sd *ScopeDetector) testSearchRead(ctx context.Context) bool {
	// Search with an empty query to test the scope
	_, _, err := sd.client.SearchContext(ctx, "test", slack.SearchParameters{
		Count: 1,
	})
	return !isMissingScopeError(err)
}

// isMissingScopeError checks if the error is a missing_scope error
func isMissingScopeError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "missing_scope") ||
		strings.Contains(errStr, "not_allowed") ||
		strings.Contains(errStr, "access_denied")
}
