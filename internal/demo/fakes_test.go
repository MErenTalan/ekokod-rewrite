package demo_test

import (
	"context"

	"github.com/google/uuid"

	"github.com/MErenTalan/ekokod-rewrite/internal/domain/model"
	"github.com/MErenTalan/ekokod-rewrite/internal/store"
)

// notFound is a CompanyGetter whose every company is missing.
type notFound struct{}

func (notFound) Get(context.Context, store.Scope, uuid.UUID) (model.Company, error) {
	return model.Company{}, store.ErrNotFound
}
