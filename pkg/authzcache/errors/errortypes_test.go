package errors

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	ce "github.com/descope/common/pkg/common/errors"
	"github.com/descope/go-sdk/descope"
	"github.com/stretchr/testify/assert"
)

func setup() context.Context {
	return context.Background()
}

func TestMalformedErrors(t *testing.T) {
	ctx := setup()
	// Error code is too short
	sdkErr := &descope.Error{
		Code:        "123",
		Description: "bla bla",
	}
	serviceErr := ServiceErrorFromSdkError(ctx, sdkErr)
	assert.True(t, ce.InternalError.Is(serviceErr))
	// Error code is empty
	sdkErr = &descope.Error{
		Code:        "",
		Description: "bla bla",
	}
	serviceErr = ServiceErrorFromSdkError(ctx, sdkErr)
	assert.True(t, ce.InternalError.Is(serviceErr))
	// nil error
	serviceErr = ServiceErrorFromSdkError(ctx, nil)
	assert.True(t, ce.InternalError.Is(serviceErr))
}

func TestServiceErrorFromSdkError_NoInfo(t *testing.T) {
	ctx := setup()
	sdkErr := &descope.Error{
		Code:        "9999",
		Description: "blappy bloop",
	}
	expectedErr := Builder.NewExportedInternalErrorType(sdkErr.Code, sdkErr.Description)
	// nil info
	serviceErr := ServiceErrorFromSdkError(ctx, sdkErr)
	assert.True(t, expectedErr.Is(serviceErr))
	// empty info
	sdkErr.Info = make(map[string]any)
	serviceErr = ServiceErrorFromSdkError(ctx, sdkErr)
	assert.True(t, expectedErr.Is(serviceErr))
}

func TestErrorMappingFromSdkError(t *testing.T) {
	var tests = []struct {
		statusCode int
		expected   ce.ServiceErrorType
	}{
		{http.StatusBadRequest, Builder.NewExportedBadRequestErrorType("9999", "Bad Request")},
		{http.StatusUnauthorized, Builder.NewExportedAuthErrorType("9999", "Unauthorized")},
		{http.StatusForbidden, Builder.NewExportedForbiddenErrorType("9999", "Forbidden")},
		{http.StatusNotFound, Builder.NewExportedNotFoundErrorType("9999", "Not Found")},
		{http.StatusTooManyRequests, Builder.NewExportedTooManyRequestsErrorType("9999", "Too Many Requests")},
		{999, Builder.NewExportedTooManyRequestsErrorType("9999", "Unkown http status code from SDK")},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.statusCode), func(t *testing.T) {
			ctx := setup()
			infoMap := make(map[string]any)
			infoMap[descope.ErrorInfoKeys.HTTPResponseStatusCode] = tt.statusCode
			sdkErr := &descope.Error{
				Code:        "9999",
				Description: "error description",
				Info:        infoMap,
			}
			serviceErr := ServiceErrorFromSdkError(ctx, sdkErr)
			assert.True(t, tt.expected.Is(serviceErr))
		})
	}
}
