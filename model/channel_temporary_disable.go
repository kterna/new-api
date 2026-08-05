package model

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

const DefaultChannelFailureSwitchStatusCodes = "300-599"

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
	return RecordChannelTemporaryFailureWithSetting(channelID, err, dto.ChannelSettings{})
}

func RecordChannelTemporaryFailureWithSetting(channelID int, err *types.NewAPIError, setting dto.ChannelSettings) bool {
	if channelID <= 0 || !channelTemporaryDisableUsable() || !IsChannelTemporaryFailureWithSetting(err, setting) {
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

func IsChannelTemporaryFailure(err *types.NewAPIError) bool {
	return IsChannelTemporaryFailureWithSetting(err, dto.ChannelSettings{})
}

func IsChannelTemporaryFailureWithSetting(err *types.NewAPIError, setting dto.ChannelSettings) bool {
	if err == nil {
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if !ChannelFailureSwitchEnabled(setting) {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}

	switch err.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeAwsInvokeError:
		return true
	}

	if ChannelFailureSwitchMatchesStatusCode(setting, err.StatusCode) {
		return true
	}
	return false
}

func ChannelFailureSwitchEnabled(setting dto.ChannelSettings) bool {
	return setting.UpstreamFailureSwitchEnabled == nil || *setting.UpstreamFailureSwitchEnabled
}

func ChannelFailureSwitchStatusCodes(setting dto.ChannelSettings) string {
	if setting.UpstreamFailureSwitchStatusCodes == "" {
		return DefaultChannelFailureSwitchStatusCodes
	}
	return setting.UpstreamFailureSwitchStatusCodes
}

func ChannelFailureSwitchMatchesStatusCode(setting dto.ChannelSettings, statusCode int) bool {
	if statusCode < 100 || statusCode > 599 {
		return false
	}
	statusCodes := ChannelFailureSwitchStatusCodes(setting)
	ranges, err := operation_setting.ParseHTTPStatusCodeRanges(statusCodes)
	if err != nil {
		ranges, _ = operation_setting.ParseHTTPStatusCodeRanges(DefaultChannelFailureSwitchStatusCodes)
	}
	for _, r := range ranges {
		if statusCode >= r.Start && statusCode <= r.End {
			return true
		}
	}
	return false
}

// TemporaryDisableStateInfo is the public API response for a channel's temporary disable state.
type TemporaryDisableStateInfo struct {
	ChannelID     int   `json:"channel_id"`
	Disabled      bool  `json:"disabled"`
	DisabledUntil int64 `json:"disabled_until"` // unix timestamp, 0 if not disabled
	FailureCount  int   `json:"failure_count"`
	DisableCount  int   `json:"disable_count"`
	LastFailure   int64 `json:"last_failure"` // unix timestamp, 0 if never
	WindowStart   int64 `json:"window_start"` // unix timestamp
}

// GetTemporaryDisableStates returns the state of all tracked channels.
func GetTemporaryDisableStates() []TemporaryDisableStateInfo {
	channelTemporaryDisableLock.Lock()
	defer channelTemporaryDisableLock.Unlock()

	now := channelTemporaryDisableNow()
	result := make([]TemporaryDisableStateInfo, 0, len(channelTemporaryDisableStates))
	for channelID, state := range channelTemporaryDisableStates {
		disabled := !state.disabledUntil.IsZero() && now.Before(state.disabledUntil)
		if !disabled && !state.disabledUntil.IsZero() {
			state.disabledUntil = time.Time{}
		}
		info := TemporaryDisableStateInfo{
			ChannelID:    channelID,
			Disabled:     disabled,
			FailureCount: state.failureCount,
			DisableCount: state.disableCount,
			WindowStart:  state.windowStart.Unix(),
			LastFailure:  state.lastFailure.Unix(),
		}
		if !state.disabledUntil.IsZero() {
			info.DisabledUntil = state.disabledUntil.Unix()
		}
		result = append(result, info)
	}
	return result
}

// ClearTemporaryDisableState removes the temporary disable state for a channel.
func ClearTemporaryDisableState(channelID int) {
	channelTemporaryDisableLock.Lock()
	defer channelTemporaryDisableLock.Unlock()
	delete(channelTemporaryDisableStates, channelID)
	common.SysLog(fmt.Sprintf("channel #%d temporary disable state manually cleared", channelID))
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
