package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProjectID = "P2rq4u0gKUmPDQJJT3V0AqdvsUQE"

func TestSDKCallsUseCallerKeyOnlyWhenSet(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	origBaseURL, origKey := baseURL, managementKey
	baseURL, managementKey = srv.URL, "service-key"
	defer func() { baseURL, managementKey = origBaseURL, origKey }()

	mgmt, err := NewDescopeClientWithProjectID(testProjectID, nil)
	require.NoError(t, err)

	_, err = mgmt.Authz().GetModified(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+testProjectID+":service-key", gotAuth)

	_, err = mgmt.Authz().GetModified(WithCallerKey(context.Background(), "caller-key"), time.Now())
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+testProjectID+":caller-key", gotAuth)

	// an emptied caller key falls back to the service key, used by background polling
	_, err = mgmt.Authz().GetModified(WithCallerKey(WithCallerKey(context.Background(), "caller-key"), ""), time.Now())
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+testProjectID+":service-key", gotAuth)
}

func TestNewHTTPClientTLSVerification(t *testing.T) {
	origBaseURL := baseURL
	defer func() { baseURL = origBaseURL }()
	for url, wantSkip := range map[string]bool{"": false, "https://api.descope.com": false, "https://localhost:8443": true} {
		baseURL = url
		transport := newHTTPClient(testProjectID).Transport.(*callerKeyTransport).base.(*http.Transport)
		assert.Equal(t, wantSkip, transport.TLSClientConfig.InsecureSkipVerify, url)
	}
}
