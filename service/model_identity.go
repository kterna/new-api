package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	upstreamModelHeader      = "X-Upstream-Reported-Model"
	upstreamModelStreamField = "_upstream_reported_model"
	upstreamModelContextKey  = "upstream_reported_model"
	maxModelNameLength       = 256
	maxObservedBodyLength    = 1024 * 1024
)

type modelObservation struct {
	model    string
	priority int
}

// A response adapter can report the raw upstream model through the generic
// header. Otherwise New API observes the standard model field in JSON bodies.
func ObserveUpstreamModelResponse(c *gin.Context, resp *http.Response, isStream bool) {
	if c == nil || resp == nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return
	}
	if model := resp.Header.Get(upstreamModelHeader); model != "" {
		setUpstreamModel(c, model, 2)
		resp.Header.Del(upstreamModelHeader)
	}
	if isStream || resp.Body == nil || !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
		return
	}
	resp.Body = &modelObservingBody{ReadCloser: resp.Body, ctx: c}
}

type modelObservingBody struct {
	io.ReadCloser
	ctx      *gin.Context
	contents []byte
	overflow bool
}

func (body *modelObservingBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	if n > 0 && !body.overflow {
		if len(body.contents)+n > maxObservedBodyLength {
			body.contents = nil
			body.overflow = true
		} else {
			body.contents = append(body.contents, p[:n]...)
		}
	}
	if err == io.EOF && !body.overflow {
		captureStandardModel(body.ctx, body.contents)
		body.contents = nil
	}
	return n, err
}

// CaptureUpstreamModelStreamData reads standard Chat, Responses, Claude, and
// Gemini model fields. A converter may additionally preserve a model that its
// normalized response would otherwise lose in the private extension field.
func CaptureUpstreamModelStreamData(c *gin.Context, data string) string {
	if c == nil || !strings.Contains(data, "model") {
		return data
	}
	var event map[string]json.RawMessage
	if common.Unmarshal([]byte(data), &event) != nil {
		return data
	}
	if raw, found := event[upstreamModelStreamField]; found {
		delete(event, upstreamModelStreamField)
		var model string
		if common.Unmarshal(raw, &model) == nil {
			setUpstreamModel(c, model, 2)
		}
		cleaned, err := common.Marshal(event)
		if err == nil {
			data = string(cleaned)
		}
	}
	if _, found := UpstreamReportedModel(c); !found {
		captureStandardModel(c, []byte(data))
	}
	return data
}

func captureStandardModel(c *gin.Context, data []byte) {
	var value struct {
		Model        string `json:"model"`
		ModelVersion string `json:"modelVersion"`
		Response     struct {
			Model        string `json:"model"`
			ModelVersion string `json:"modelVersion"`
		} `json:"response"`
		Message struct {
			Model string `json:"model"`
		} `json:"message"`
	}
	if common.Unmarshal(data, &value) != nil {
		return
	}
	for _, model := range []string{
		value.Response.ModelVersion,
		value.ModelVersion,
		value.Response.Model,
		value.Message.Model,
		value.Model,
	} {
		if model != "" {
			setUpstreamModel(c, model, 1)
			return
		}
	}
}

func setUpstreamModel(c *gin.Context, model string, priority int) {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > maxModelNameLength || strings.IndexFunc(model, unicode.IsControl) >= 0 {
		return
	}
	if previous, found := c.Get(upstreamModelContextKey); found {
		if observation, ok := previous.(modelObservation); ok && observation.priority >= priority {
			return
		}
	}
	c.Set(upstreamModelContextKey, modelObservation{model: model, priority: priority})
}

func UpstreamReportedModel(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	value, found := c.Get(upstreamModelContextKey)
	if !found {
		return "", false
	}
	observation, ok := value.(modelObservation)
	return observation.model, ok
}
