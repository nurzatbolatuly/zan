package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/template"
)

// seededContractTypeID — «Договор» из сида миграции 000011.
const seededContractTypeID = "00000000-0000-0000-0001-000000000001"

func newTestTemplate(typeID string, now time.Time) domain.DocumentTemplate {
	id := uuid.NewString()
	return domain.DocumentTemplate{
		ID:               id,
		DocumentTypeID:   typeID,
		Title:            "Договор аренды",
		ObjectKey:        "templates/" + id,
		OriginalName:     "lease.docx",
		MimeType:         "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		SizeBytes:        2048,
		PreviewObjectKey: "converted/" + id + ".pdf",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func TestTemplateRepo_ListTypes_SeededByMigrationInAlphabeticalOrder(t *testing.T) {
	r := repo.NewTemplateRepo(setupTestDB(t))

	types, err := r.ListTypes(context.Background())

	require.NoError(t, err)
	require.Len(t, types, 7)
	require.Equal(t, "Доверенность", types[0].Name)
}

func TestTemplateRepo_TypeNameIsUniqueIgnoringCase(t *testing.T) {
	r := repo.NewTemplateRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := r.CreateType(ctx, domain.DocumentType{ID: uuid.NewString(), Name: "ДОГОВОР", CreatedAt: now, UpdatedAt: now})
	require.ErrorIs(t, err, template.ErrTypeNameTaken)

	created, err := r.CreateType(ctx, domain.DocumentType{ID: uuid.NewString(), Name: "Акт", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)

	_, err = r.RenameType(ctx, created.ID, "приказ", now)
	require.ErrorIs(t, err, template.ErrTypeNameTaken)

	renamed, err := r.RenameType(ctx, created.ID, "Акт приёма-передачи", now)
	require.NoError(t, err)
	require.Equal(t, "Акт приёма-передачи", renamed.Name)
}

func TestTemplateRepo_TemplateLifecycle(t *testing.T) {
	r := repo.NewTemplateRepo(setupTestDB(t))
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := r.CreateTemplate(ctx, newTestTemplate(seededContractTypeID, now))
	require.NoError(t, err)

	// Тип с шаблоном удалить нельзя — FK без CASCADE.
	require.ErrorIs(t, r.DeleteType(ctx, seededContractTypeID), template.ErrTypeInUse)

	next := created
	next.Title = "Договор аренды квартиры"
	next.DocumentTypeID = "00000000-0000-0000-0001-000000000002"
	next.UpdatedAt = now.Add(time.Minute)
	updated, err := r.UpdateTemplate(ctx, next)
	require.NoError(t, err)
	require.Equal(t, next.Title, updated.Title)
	require.Equal(t, next.DocumentTypeID, updated.DocumentTypeID)

	listed, err := r.ListTemplates(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, created.PreviewObjectKey, listed[0].PreviewObjectKey)

	require.NoError(t, r.DeleteTemplate(ctx, created.ID))
	_, err = r.GetTemplate(ctx, created.ID)
	require.ErrorIs(t, err, template.ErrNotFound)
	require.ErrorIs(t, r.DeleteTemplate(ctx, created.ID), template.ErrNotFound)
	require.NoError(t, r.DeleteType(ctx, seededContractTypeID))
}

func TestTemplateRepo_CreateTemplateWithUnknownTypeReturnsTypeNotFound(t *testing.T) {
	r := repo.NewTemplateRepo(setupTestDB(t))

	_, err := r.CreateTemplate(context.Background(), newTestTemplate(uuid.NewString(), time.Now().UTC()))

	require.ErrorIs(t, err, template.ErrTypeNotFound)
}
