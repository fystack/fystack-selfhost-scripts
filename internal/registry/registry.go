package registry

import (
	"context"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type TagLister interface {
	Tags(ctx context.Context, image string) ([]string, error)
}

type RemoteTagLister struct{}

func (RemoteTagLister) Tags(ctx context.Context, image string) ([]string, error) {
	ref, err := name.ParseReference(image)
	if err != nil {
		return nil, err
	}
	return remote.List(ref.Context(), remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
}

func ImageTag(image string) (string, bool) {
	idx := strings.LastIndex(image, ":")
	if idx < 0 || idx == len(image)-1 {
		return "", false
	}
	if slash := strings.LastIndex(image, "/"); slash > idx {
		return "", false
	}
	return image[idx+1:], true
}
