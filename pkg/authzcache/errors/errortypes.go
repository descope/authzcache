package errors

import (
	"context"
	"fmt"
	"net/http"

	cctx "github.com/descope/common/pkg/common/context"
	ce "github.com/descope/common/pkg/common/errors"
	"github.com/descope/go-sdk/descope"
)

const errorServiceID = "17"

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
