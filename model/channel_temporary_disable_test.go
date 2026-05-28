package model

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/types"
)

func setupChannelTemporaryDisableTest(t *testing.T) *time.Time {
	t.Helper()

	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)

	channelTemporaryDisableLock.Lock()
	oldConfig := channelTemporaryDisableConfig
	oldNow := channelTemporaryDisableNow
	oldStates := channelTemporaryDisableStates
	channelTemporaryDisableConfig = channelTemporaryDisableSettings{
		Enabled:          true,
		FailureThreshold: 3,
		FailureWindow:    time.Minute,
		Cooldown:         2 * time.Minute,
		MaxCooldown:      10 * time.Minute,
	}
	channelTemporaryDisableNow = func() time.Time { return now }
	channelTemporaryDisableStates = make(map[int]*channelTemporaryDisableState)
	channelTemporaryDisableLock.Unlock()

	t.Cleanup(func() {
		channelTemporaryDisableLock.Lock()
		channelTemporaryDisableConfig = oldConfig
		channelTemporaryDisableNow = oldNow
		channelTemporaryDisableStates = oldStates
		channelTemporaryDisableLock.Unlock()
	})

	return &now
}

func upstreamFailure(statusCode int) *types.NewAPIError {
	return types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, statusCode)
}

func TestRecordChannelTemporaryFailureOpensAfterThreshold(t *testing.T) {
	now := setupChannelTemporaryDisableTest(t)

	if RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusGatewayTimeout)) {
		t.Fatal("first failure should not open temporary disable")
	}
	if RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusGatewayTimeout)) {
		t.Fatal("second failure should not open temporary disable")
	}
	if !RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusGatewayTimeout)) {
		t.Fatal("third failure should open temporary disable")
	}
	if !IsChannelTemporarilyDisabled(7) {
		t.Fatal("channel should be temporarily disabled")
	}

	filtered := filterTemporarilyDisabledChannelIDs([]int{7, 8})
	if !reflect.DeepEqual(filtered, []int{8}) {
		t.Fatalf("unexpected filtered channel ids: %#v", filtered)
	}

	*now = now.Add(2*time.Minute + time.Second)
	if IsChannelTemporarilyDisabled(7) {
		t.Fatal("temporary disable should expire after cooldown")
	}
}

func TestRecordChannelTemporaryFailureResetsOutsideWindow(t *testing.T) {
	now := setupChannelTemporaryDisableTest(t)

	RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusBadGateway))
	RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusBadGateway))
	*now = now.Add(time.Minute + time.Second)

	if RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusBadGateway)) {
		t.Fatal("failure after window should start a new window")
	}
	if RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusBadGateway)) {
		t.Fatal("second failure in new window should not open temporary disable")
	}
	if !RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusBadGateway)) {
		t.Fatal("third failure in new window should open temporary disable")
	}
}

func TestRecordChannelTemporarySuccessClearsFailureState(t *testing.T) {
	setupChannelTemporaryDisableTest(t)

	RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusInternalServerError))
	RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusInternalServerError))
	RecordChannelTemporarySuccess(7)

	if RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusInternalServerError)) {
		t.Fatal("success should clear previous failure count")
	}
}

func TestSkipRetryLocalErrorDoesNotOpenTemporaryDisable(t *testing.T) {
	setupChannelTemporaryDisableTest(t)

	localErr := types.NewErrorWithStatusCode(
		errors.New("invalid local request"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)

	for i := 0; i < 5; i++ {
		if RecordChannelTemporaryFailure(7, localErr) {
			t.Fatal("skip-retry local error should not open temporary disable")
		}
	}
	if IsChannelTemporarilyDisabled(7) {
		t.Fatal("skip-retry local error should not mark channel temporarily disabled")
	}
}

func TestUpstreamBadRequestCanOpenTemporaryDisable(t *testing.T) {
	setupChannelTemporaryDisableTest(t)

	upstreamBadRequest := types.WithOpenAIError(types.OpenAIError{
		Message: "upstream bad request",
		Type:    "invalid_request_error",
		Code:    "invalid_request_error",
	}, http.StatusBadRequest)

	if RecordChannelTemporaryFailure(7, upstreamBadRequest) {
		t.Fatal("first upstream 400 should not open temporary disable")
	}
	if RecordChannelTemporaryFailure(7, upstreamBadRequest) {
		t.Fatal("second upstream 400 should not open temporary disable")
	}
	if !RecordChannelTemporaryFailure(7, upstreamBadRequest) {
		t.Fatal("third upstream 400 should open temporary disable")
	}
}

func TestChannelTemporaryDisableCooldownEscalates(t *testing.T) {
	now := setupChannelTemporaryDisableTest(t)

	openTemporaryDisable := func() {
		t.Helper()
		for i := 0; i < channelTemporaryDisableConfig.FailureThreshold; i++ {
			RecordChannelTemporaryFailure(7, upstreamFailure(http.StatusBadGateway))
		}
	}
	remainingCooldown := func() time.Duration {
		t.Helper()
		channelTemporaryDisableLock.Lock()
		defer channelTemporaryDisableLock.Unlock()
		state := channelTemporaryDisableStates[7]
		if state == nil {
			t.Fatal("missing channel temporary disable state")
		}
		return state.disabledUntil.Sub(*now)
	}

	openTemporaryDisable()
	if got := remainingCooldown(); got != 2*time.Minute {
		t.Fatalf("first cooldown = %s, want 2m", got)
	}

	*now = now.Add(2*time.Minute + time.Second)
	if IsChannelTemporarilyDisabled(7) {
		t.Fatal("first cooldown should be expired")
	}

	openTemporaryDisable()
	if got := remainingCooldown(); got != 4*time.Minute {
		t.Fatalf("second cooldown = %s, want 4m", got)
	}

	RecordChannelTemporarySuccess(7)
	openTemporaryDisable()
	if got := remainingCooldown(); got != 2*time.Minute {
		t.Fatalf("success should reset cooldown, got %s", got)
	}
}
