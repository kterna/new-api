package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandardResponseModelIsLoggedForAnyChannel(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	response := `{"id":"chatcmpl-1","model":"gpt-6-luna"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(response)),
	}
	ObserveUpstreamModelResponse(ctx, resp, false)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, response, string(body))

	model, found := UpstreamReportedModel(ctx)
	require.True(t, found)
	assert.Equal(t, "gpt-6-luna", model)

	now := time.Now()
	info := &relaycommon.RelayInfo{
		OriginModelName:   "gpt-6-astra",
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, 0, 1)
	admin, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "gpt-6-luna", admin["upstream_reported_model"])
	assert.Equal(t, "gpt-6-astra", info.OriginModelName)
}

func TestExplicitUpstreamModelOverridesConvertedResponseModel(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	header := http.Header{}
	header.Set(upstreamModelHeader, "gemini-3.1-pro-high")
	header.Set("Content-Type", "application/json")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(`{"model":"gemini-pro-agent"}`)),
	}
	ObserveUpstreamModelResponse(ctx, resp, false)
	_, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	model, found := UpstreamReportedModel(ctx)
	require.True(t, found)
	assert.Equal(t, "gemini-3.1-pro-high", model)
	assert.Empty(t, resp.Header.Get(upstreamModelHeader))
	assert.False(t, ShouldCopyUpstreamHeader(nil, upstreamModelHeader, nil))
}

func TestMissingOrInvalidResponseModelIsNotInvented(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"usage":{"total_tokens":1}}`)),
	}
	ObserveUpstreamModelResponse(ctx, resp, false)
	_, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	_, found := UpstreamReportedModel(ctx)
	assert.False(t, found)

	resp.Header.Set(upstreamModelHeader, "invalid\nmodel")
	ObserveUpstreamModelResponse(ctx, resp, false)
	_, found = UpstreamReportedModel(ctx)
	assert.False(t, found)
}

func TestStandardModelFieldsAcrossResponseFormats(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"responses", `{"type":"response.created","response":{"model":"gpt-6-luna"}}`, "gpt-6-luna"},
		{"claude", `{"type":"message_start","message":{"model":"claude-sonnet-4-6"}}`, "claude-sonnet-4-6"},
		{"gemini", `{"modelVersion":"gemini-3.1-pro-high"}`, "gemini-3.1-pro-high"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			captureStandardModel(ctx, []byte(testCase.body))
			model, found := UpstreamReportedModel(ctx)
			require.True(t, found)
			assert.Equal(t, testCase.want, model)
		})
	}
}
