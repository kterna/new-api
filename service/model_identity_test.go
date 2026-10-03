package service

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCPAModelIdentityIsLoggedSeparatelyFromRequestedModel(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	report := `{"requested_model":"gemini-pro-agent","routed_model":"gemini-pro-agent","reported_model":"gemini-3.1-pro-high","comparison":"same","source":"response.modelVersion","response_id":"resp_123"}`
	headers := http.Header{}
	headers.Set(cpaModelIdentityHeader, base64.RawURLEncoding.EncodeToString([]byte(report)))
	CaptureCPAModelIdentityHeader(ctx, headers)

	now := time.Now()
	info := &relaycommon.RelayInfo{
		OriginModelName:   "public-gemini-alias",
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}
	other := GenerateTextOtherInfo(ctx, info, 1, 1, 1, 0, 0, 0, 1)
	admin, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	identity, ok := admin["cpa_model_identity"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "public-gemini-alias", identity["newapi_requested_model"])
	assert.Equal(t, "gemini-pro-agent", identity["cpa_requested_model"])
	assert.Equal(t, "gemini-3.1-pro-high", identity["upstream_reported_model"])
	assert.Equal(t, "different", identity["comparison"])
}

func TestCPAModelIdentityCommentIgnoresMalformedObservation(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.True(t, CaptureCPAModelIdentityComment(ctx, cpaModelIdentityComment+"not-base64"))
	_, found := cpaModelIdentity(ctx)
	assert.False(t, found)
	assert.False(t, CaptureCPAModelIdentityComment(ctx, "data: {}"))
}

func TestCPAModelIdentityHeaderStaysInsideNewAPI(t *testing.T) {
	assert.False(t, ShouldCopyUpstreamHeader(nil, "x-cpa-model-identity", nil))
	assert.True(t, ShouldCopyUpstreamHeader(nil, "X-Other-Header", nil))
}
