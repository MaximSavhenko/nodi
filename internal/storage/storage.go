package storage

import (
	"context"
	"io"
	"nodi/internal/model"
)

// Store keeps user commands independent from the storage provider.
type Store interface {
	Index(context.Context, bool) ([]Version, error)
	State(context.Context, string) (ProjectState, error)
	List(context.Context, bool) ([]model.Manifest, error)
	ProjectList(context.Context, string, bool) ([]model.Manifest, error)
	Get(context.Context, string, string) (model.Manifest, error)
	Download(context.Context, model.Manifest, io.Writer) error
	Publish(context.Context, model.Manifest, string) error
}
