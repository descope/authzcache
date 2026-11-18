package remote

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/descope/go-sdk/descope/api"
)

type callerKeyCtxKey struct{}

// WithCallerKey makes SDK calls made with ctx authenticate with the caller's management key
func WithCallerKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, callerKeyCtxKey{}, key)
}

func CallerKey(ctx context.Context) string {
	key, _ := ctx.Value(callerKeyCtxKey{}).(string)
	return key
}

// callerKeyTransport swaps the service key for the caller's key, when one is set on the request context
type callerKeyTransport struct {
	projectID string
	base      http.RoundTripper
}

func (t *callerKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if key := CallerKey(req.Context()); key != "" {
		req = req.Clone(req.Context())
		req.Header.Set(api.AuthorizationHeaderName, api.BearerAuthorizationPrefix+t.projectID+":"+key)
	}
	return t.base.RoundTrip(req)
}

// newHTTPClient mirrors the go-sdk default client, with the caller key transport on top
func newHTTPClient(projectID string) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxConnsPerHost = 100
	t.MaxIdleConnsPerHost = 100
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{} // notest
	}
	// an empty base URL resolves to a production Descope URL, always verified
	t.TLSClientConfig.InsecureSkipVerify = baseURL != "" && api.CertificateVerifyAutomatic.SkipVerifyValue(baseURL) //nolint:gosec
	return &http.Client{
		Timeout:   60 * time.Second,
		Transport: &callerKeyTransport{projectID: projectID, base: t},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // notest
		},
	}
}
