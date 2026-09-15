package errors

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	cctx "github.com/descope/backend/common/pkg/common/context"
	ce "github.com/descope/backend/common/pkg/common/errors"
	"github.com/descope/go-sdk/descope"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const errorServiceID = "17"

// RetryAfterHeader relays Descope's backoff to callers behind the cache
const RetryAfterHeader = "Retry-After"

var Builder = ce.NewErrorTypeBuilder(errorServiceID)

func ServiceErrorFromSdkError(ctx context.Context, err error) ce.ServiceError {
	if de, ok := err.(*descope.Error); ok && len(de.Code) >= 4 {
		code := de.Code[len(de.Code)-4:]
		statusCode := de.Info[descope.ErrorInfoKeys.HTTPResponseStatusCode]
		var builder ce.ServiceErrorType
		switch statusCode {
		case http.StatusBadRequest:
			builder = Builder.NewExportedBadRequestErrorType(code, de.Description)
		case http.StatusUnauthorized:
			builder = Builder.NewExportedAuthErrorType(code, de.Description)
		case http.StatusForbidden:
			builder = Builder.NewExportedForbiddenErrorType(code, de.Description)
		case http.StatusNotFound:
			builder = Builder.NewExportedNotFoundErrorType(code, de.Description)
		case http.StatusTooManyRequests:
			setRetryAfter(ctx, de)
			builder = Builder.NewExportedTooManyRequestsErrorType(code, de.Description)
		default:
			cctx.Logger(ctx).Error().Msg(fmt.Sprintf("Unknown http status code from SDK: %v\n", statusCode))
			builder = Builder.NewExportedInternalErrorType(code, de.Description)
		}
		return builder.NewErrorWithLogLevelWarn(ctx, de, de.Message)
	}
	// got malformed or non descope error
	if err == nil {
		return ce.InternalError.NewErrorWithLog(ctx, err, "nil error")
	}
	return ce.InternalError.NewErrorWithLog(ctx, err, err.Error())
}

// setRetryAfter passes the SDK's parsed Retry-After seconds on as grpc header metadata for the gateway error handler
func setRetryAfter(ctx context.Context, de *descope.Error) {
	seconds, ok := de.Info[descope.ErrorInfoKeys.RateLimitExceededRetryAfter].(int)
	if !ok || seconds <= 0 {
		return
	}
	if err := grpc.SetHeader(ctx, metadata.Pairs(RetryAfterHeader, strconv.Itoa(seconds))); err != nil {
		cctx.Logger(ctx).Warn().Err(err).Msg("Failed to set Retry-After header metadata")
	}
}
