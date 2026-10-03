package service

import (
	"encoding/base64"
	"net/http"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	cpaModelIdentityHeader  = "X-Cpa-Model-Identity"
	cpaModelIdentityComment = ": cpa-model-identity "
	cpaModelIdentityKey     = "cpa_model_identity"
	maxIdentityLength       = 2048
)

// CPAModelIdentity records what an upstream reported. A different identifier
// may be an intentional alias, so this must not affect routing or billing.
type CPAModelIdentity struct {
	RequestedModel string `json:"requested_model"`
	RoutedModel    string `json:"routed_model,omitempty"`
	ReportedModel  string `json:"reported_model"`
	Comparison     string `json:"comparison"`
	Source         string `json:"source"`
	ResponseID     string `json:"response_id,omitempty"`
}

func CaptureCPAModelIdentityHeader(c *gin.Context, headers http.Header) {
	if c == nil || headers == nil {
		return
	}
	captureCPAModelIdentity(c, headers.Get(cpaModelIdentityHeader))
}

// CaptureCPAModelIdentityComment consumes the plugin's SSE comment. It returns
// true for malformed observations too, so these internal comments never reach
// a client as model output.
func CaptureCPAModelIdentityComment(c *gin.Context, line string) bool {
	if !strings.HasPrefix(line, cpaModelIdentityComment) {
		return false
	}
	captureCPAModelIdentity(c, strings.TrimSpace(strings.TrimPrefix(line, cpaModelIdentityComment)))
	return true
}

func captureCPAModelIdentity(c *gin.Context, encoded string) {
	if c == nil || encoded == "" || len(encoded) > maxIdentityLength {
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(raw) > maxIdentityLength {
		return
	}
	var identity CPAModelIdentity
	if common.Unmarshal(raw, &identity) != nil ||
		!validIdentityValue(identity.RequestedModel, 256) ||
		!validIdentityValue(identity.RoutedModel, 256) ||
		!validIdentityValue(identity.ReportedModel, 256) ||
		!validIdentityValue(identity.Source, 64) ||
		!validIdentityValue(identity.ResponseID, 256) ||
		identity.ReportedModel == "" {
		return
	}
	identity.Comparison = "different"
	if strings.EqualFold(identity.RequestedModel, identity.ReportedModel) {
		identity.Comparison = "same"
	}
	c.Set(cpaModelIdentityKey, identity)
}

func validIdentityValue(value string, maxLength int) bool {
	if len(value) > maxLength {
		return false
	}
	return strings.IndexFunc(value, unicode.IsControl) < 0
}

func cpaModelIdentity(c *gin.Context) (CPAModelIdentity, bool) {
	if c == nil {
		return CPAModelIdentity{}, false
	}
	value, found := c.Get(cpaModelIdentityKey)
	if !found {
		return CPAModelIdentity{}, false
	}
	identity, ok := value.(CPAModelIdentity)
	return identity, ok
}
