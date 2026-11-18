package services

import (
	"context"
	"errors"
	"sync"
	"time"

	cctx "github.com/descope/backend/common/pkg/common/context"
	"github.com/descope/go-sdk/descope"
)

type callerKey struct {
	projectID string
	key       string
}

type keyVerifyFunc func(ctx context.Context, projectID, key string) error

// keyVerifier remembers caller keys Descope accepted, and revalidates them periodically instead of a TTL
type keyVerifier struct {
	verified sync.Map // callerKey -> struct{}
	verify   keyVerifyFunc
}

// ponytail: concurrent first requests with the same key each verify, add singleflight if that load matters
func (v *keyVerifier) ensure(ctx context.Context, projectID, key string) error {
	k := callerKey{projectID: projectID, key: key}
	if _, ok := v.verified.Load(k); ok {
		return nil
	}
	if err := v.verify(ctx, projectID, key); err != nil {
		return err
	}
	v.verified.Store(k, struct{}{})
	return nil
}

func (v *keyVerifier) markVerified(projectID, key string) {
	v.verified.Store(callerKey{projectID: projectID, key: key}, struct{}{})
}

func (v *keyVerifier) revalidate(ctx context.Context) {
	v.verified.Range(func(k, _ any) bool {
		ck := k.(callerKey)
		if err := v.verify(ctx, ck.projectID, ck.key); isKeyRejected(err) {
			cctx.Logger(ctx).Info().Str("project_id", ck.projectID).Msg("Dropping caller key rejected on revalidation")
			v.verified.Delete(k)
		}
		return true
	})
}

func (v *keyVerifier) startRevalidation(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	cctx.Go(ctx, func(ctx context.Context) {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				v.revalidate(ctx)
			}
		}
	})
}

// only an explicit rejection drops a key, so a Descope outage doesn't fail cache hits
func isKeyRejected(err error) bool {
	var de *descope.Error
	return errors.As(err, &de) && (de.IsUnauthorized() || de.IsForbidden())
}
