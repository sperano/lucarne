package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// classify wraps an error of the Kubernetes API in the source's own: the
// context's error when the caller gave up, ErrForbidden when the
// credentials were refused, ErrUnavailable otherwise.
func classify(ctx context.Context, err error, what string) error {
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("%s: %w", what, ctx.Err())
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return fmt.Errorf("%s: %w: %w", what, ErrForbidden, err)
	default:
		return fmt.Errorf("%s: %w: %w", what, ErrUnavailable, err)
	}
}

func isNotFound(err error) bool { return apierrors.IsNotFound(err) }
