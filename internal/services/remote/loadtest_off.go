//go:build !loadtest

package remote

import "github.com/descope/go-sdk/descope/sdk"

// fakeClient is a no-op in production builds; the load-test fake only exists under the loadtest tag.
func fakeClient(_ string) (sdk.Management, bool) {
	return nil, false
}
