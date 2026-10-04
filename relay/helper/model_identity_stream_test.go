package helper

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamScannerCapturesGenericModelExtensionWithoutForwardingIt(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	body := "data: {\"type\":\"response.created\",\"response\":{\"model\":\"gemini-pro-agent\"},\"_upstream_reported_model\":\"gemini-3.1-pro-high\"}\n\n" +
		"data: [DONE]\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{StartTime: time.Now()}

	var received []string
	StreamScannerHandler(ctx, resp, info, func(data string, _ *StreamResult) {
		received = append(received, data)
	})

	require.Len(t, received, 1)
	assert.NotContains(t, received[0], "_upstream_reported_model")
	assert.Contains(t, received[0], "gemini-pro-agent")
	model, found := service.UpstreamReportedModel(ctx)
	require.True(t, found)
	assert.Equal(t, "gemini-3.1-pro-high", model)
}

func TestStreamScannerCapturesStandardModelWithoutPlugin(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	body := "data: {\"id\":\"chatcmpl-1\",\"model\":\"gpt-6-luna\",\"choices\":[]}\n\n" +
		"data: [DONE]\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{StartTime: time.Now()}

	StreamScannerHandler(ctx, resp, info, func(data string, _ *StreamResult) {})

	model, found := service.UpstreamReportedModel(ctx)
	require.True(t, found)
	assert.Equal(t, "gpt-6-luna", model)
}
