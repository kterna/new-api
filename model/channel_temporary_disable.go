package model

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
)

type channelTemporaryDisableState struct {
	failureCount  int
	disableCount  int
	windowStart   time.Time
	lastFailure   time.Time
	disabledUntil time.Time
}

type channelTemporaryDisableSettings struct {
	Enabled          bool
	FailureThreshold int
	FailureWindow    time.Duration
	Cooldown         time.Duration
	MaxCooldown      time.Duration
}

var (
	channelTemporaryDisableConfig = channelTemporaryDisableSettings{
		Enabled:          common.GetEnvOrDefaultBool("CHANNEL_TEMP_DISABLE_ENABLED", true),
		FailureThreshold: common.GetEnvOrDefault("CHANNEL_TEMP_DISABLE_FAILURE_THRESHOLD", 3),
		FailureWindow:    time.Duration(common.GetEnvOrDefault("CHANNEL_TEMP_DISABLE_WINDOW_SECONDS", 60)) * time.Second,
		Cooldown:         time.Duration(common.GetEnvOrDefault("CHANNEL_TEMP_DISABLE_COOLDOWN_SECONDS", 120)) * time.Second,
		MaxCooldown:      time.Duration(common.GetEnvOrDefault("CHANNEL_TEMP_DISABLE_MAX_COOLDOWN_SECONDS", 1800)) * time.Second,
	}
	channelTemporaryDisableNow    = time.Now
	channelTemporaryDisableLock   sync.Mutex
	channelTemporaryDisableStates = make(map[int]*channelTemporaryDisableState)
)

func channelTemporaryDisableUsable() bool {
	return channelTemporaryDisableConfig.Enabled &&
		channelTemporaryDisableConfig.FailureThreshold > 0 &&
		channelTemporaryDisableConfig.FailureWindow > 0 &&
		channelTemporaryDisableConfig.Cooldown > 0 &&
		channelTemporaryDisableConfig.MaxCooldown > 0
}

func IsChannelTemporarilyDisabled(channelID int) bool {
	if channelID <= 0 || !channelTemporaryDisableUsable() {
		return false
	}

	now := channelTemporaryDisableNow()
	channelTemporaryDisableLock.Lock()
	defer channelTemporaryDisableLock.Unlock()

	state := channelTemporaryDisableStates[channelID]
	if state == nil || state.disabledUntil.IsZero() {
		return false
	}
	if now.Before(state.disabledUntil) {
		return true
	}

	state.disabledUntil = time.Time{}
	common.SysLog(fmt.Sprintf("channel #%d temporary disable expired", channelID))
	return false
}

func RecordChannelTemporarySuccess(channelID int) {
	if channelID <= 0 || !channelTemporaryDisableUsable() {
		return
	}

	channelTemporaryDisableLock.Lock()
	defer channelTemporaryDisableLock.Unlock()

	if _, ok := channelTemporaryDisableStates[channelID]; ok {
		delete(channelTemporaryDisableStates, channelID)
	}
}

func RecordChannelTemporaryFailure(channelID int, err *types.NewAPIError) bool {
	if channelID <= 0 || !shouldRecordChannelTemporaryFailure(err) {
		return false
	}

	now := channelTemporaryDisableNow()
	channelTemporaryDisableLock.Lock()
	defer channelTemporaryDisableLock.Unlock()

	state := channelTemporaryDisableStates[channelID]
	if state != nil && now.Before(state.disabledUntil) {
		return true
	}
	if state == nil || now.Sub(state.windowStart) > channelTemporaryDisableConfig.FailureWindow {
		disableCount := 0
		if state != nil {
			disableCount = state.disableCount
		}
		state = &channelTemporaryDisableState{
			disableCount: disableCount,
			windowStart:  now,
		}
		channelTemporaryDisableStates[channelID] = state
	}

	state.failureCount++
	state.lastFailure = now
	if state.failureCount < channelTemporaryDisableConfig.FailureThreshold {
		return false
	}

	state.disableCount++
	cooldown := channelTemporaryDisableCooldown(state.disableCount)
	state.disabledUntil = now.Add(cooldown)
	state.failureCount = 0
	state.windowStart = now
	common.SysLog(fmt.Sprintf("channel #%d temporarily disabled for %s after repeated upstream failures: %s",
		channelID,
		cooldown,
		err.MaskSensitiveErrorWithStatusCode(),
	))
	return true
}

func channelTemporaryDisableCooldown(disableCount int) time.Duration {
	if channelTemporaryDisableConfig.Cooldown >= channelTemporaryDisableConfig.MaxCooldown {
		return channelTemporaryDisableConfig.MaxCooldown
	}
	if disableCount <= 1 {
		return channelTemporaryDisableConfig.Cooldown
	}

	cooldown := channelTemporaryDisableConfig.Cooldown
	for i := 1; i < disableCount; i++ {
		if cooldown >= channelTemporaryDisableConfig.MaxCooldown/2 {
			return channelTemporaryDisableConfig.MaxCooldown
		}
		cooldown *= 2
	}
	if cooldown > channelTemporaryDisableConfig.MaxCooldown {
		return channelTemporaryDisableConfig.MaxCooldown
	}
	return cooldown
}

func shouldRecordChannelTemporaryFailure(err *types.NewAPIError) bool {
	if err == nil || !channelTemporaryDisableUsable() {
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}

	switch err.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponseStatusCode,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeAwsInvokeError:
		return true
	}

	if err.StatusCode >= http.StatusMultipleChoices && err.StatusCode <= 599 {
		return true
	}
	return false
}

func filterTemporarilyDisabledChannelIDs(channelIDs []int) []int {
	if len(channelIDs) == 0 || !channelTemporaryDisableUsable() {
		return channelIDs
	}

	filtered := make([]int, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		if !IsChannelTemporarilyDisabled(channelID) {
			filtered = append(filtered, channelID)
		}
	}
	return filtered
}

func filterTemporarilyDisabledAbilities(abilities []Ability) []Ability {
	if len(abilities) == 0 || !channelTemporaryDisableUsable() {
		return abilities
	}

	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if !IsChannelTemporarilyDisabled(ability.ChannelId) {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}
