package helper

import (
	"encoding/base64"
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

func TestStreamScannerCapturesIdentityCommentWithoutForwardingIt(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	report := `{"requested_model":"gemini-pro-agent","reported_model":"gemini-3.1-pro-high","source":"response.modelVersion"}`
	comment := ": cpa-model-identity " + base64.RawURLEncoding.EncodeToString([]byte(report)) + "\n\n"
	body := comment + "data: {\"type\":\"response.created\"}\n\n" + "data: [DONE]\n\n"
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
	info := &relaycommon.RelayInfo{StartTime: time.Now()}

	var received []string
	StreamScannerHandler(ctx, resp, info, func(data string, _ *StreamResult) {
		received = append(received, data)
	})

	require.Equal(t, []string{`{"type":"response.created"}`}, received)
	value, found := ctx.Get("cpa_model_identity")
	require.True(t, found)
	identity, ok := value.(service.CPAModelIdentity)
	require.True(t, ok)
	assert.Equal(t, "gemini-3.1-pro-high", identity.ReportedModel)
	assert.Equal(t, "different", identity.Comparison)
}
