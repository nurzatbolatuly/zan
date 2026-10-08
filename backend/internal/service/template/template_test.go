package template_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/template"
)

const (
	testMaxSize = 1024
	typeID      = "00000000-0000-0000-0001-000000000001"
)

var (
	pdfData  = []byte("%PDF-1.7 template")
	docxData = []byte("PK\x03\x04 docx template")
)

// fakeRepo — in-memory template.Repository (BACKEND_CODING_STANDARDS.md §10:
// юниты без БД, реальная семантика ограничений — internal/repo/template_repo_test.go).
type fakeRepo struct {
	types     map[string]domain.DocumentType
	templates map[string]domain.DocumentTemplate
	writeErr  error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		types:     map[string]domain.DocumentType{typeID: {ID: typeID, Name: "Договор"}},
		templates: make(map[string]domain.DocumentTemplate),
	}
}

func (r *fakeRepo) ListTypes(context.Context) ([]domain.DocumentType, error) {
	out := make([]domain.DocumentType, 0, len(r.types))
	for _, t := range r.types {
		out = append(out, t)
	}
	return out, nil
}

func (r *fakeRepo) GetType(_ context.Context, id string) (domain.DocumentType, error) {
	t, ok := r.types[id]
	if !ok {
		return domain.DocumentType{}, template.ErrTypeNotFound
	}
	return t, nil
}

func (r *fakeRepo) CreateType(_ context.Context, t domain.DocumentType) (domain.DocumentType, error) {
	for _, existing := range r.types {
		if strings.EqualFold(existing.Name, t.Name) {
			return domain.DocumentType{}, template.ErrTypeNameTaken
		}
	}
	r.types[t.ID] = t
	return t, nil
}

func (r *fakeRepo) RenameType(_ context.Context, id, name string, updatedAt time.Time) (domain.DocumentType, error) {
	t, ok := r.types[id]
	if !ok {
		return domain.DocumentType{}, template.ErrTypeNotFound
	}
	t.Name, t.UpdatedAt = name, updatedAt
	r.types[id] = t
	return t, nil
}

func (r *fakeRepo) DeleteType(_ context.Context, id string) error {
	for _, tpl := range r.templates {
		if tpl.DocumentTypeID == id {
			return template.ErrTypeInUse
		}
	}
	delete(r.types, id)
	return nil
}

func (r *fakeRepo) ListTemplates(context.Context) ([]domain.DocumentTemplate, error) {
	out := make([]domain.DocumentTemplate, 0, len(r.templates))
	for _, t := range r.templates {
		out = append(out, t)
	}
	return out, nil
}

func (r *fakeRepo) GetTemplate(_ context.Context, id string) (domain.DocumentTemplate, error) {
	t, ok := r.templates[id]
	if !ok {
		return domain.DocumentTemplate{}, template.ErrNotFound
	}
	return t, nil
}

func (r *fakeRepo) CreateTemplate(_ context.Context, t domain.DocumentTemplate) (domain.DocumentTemplate, error) {
	if r.writeErr != nil {
		return domain.DocumentTemplate{}, r.writeErr
	}
	r.templates[t.ID] = t
	return t, nil
}

func (r *fakeRepo) UpdateTemplate(_ context.Context, t domain.DocumentTemplate) (domain.DocumentTemplate, error) {
	if r.writeErr != nil {
		return domain.DocumentTemplate{}, r.writeErr
	}
	r.templates[t.ID] = t
	return t, nil
}

func (r *fakeRepo) DeleteTemplate(_ context.Context, id string) error {
	delete(r.templates, id)
	return nil
}

// fakeStorage — множество ключей, реально лежащих в бакете.
type fakeStorage struct {
	objects map[string]bool
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{objects: make(map[string]bool)}
}

func (s *fakeStorage) Put(_ context.Context, key string, _ io.Reader, _ int64, _ string) error {
	s.objects[key] = true
	return nil
}

func (s *fakeStorage) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func (s *fakeStorage) PresignGetPublic(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.public/" + key, nil
}

func (s *fakeStorage) PresignGetInternal(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.internal/" + key, nil
}

func (s *fakeStorage) keys() []string {
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// fakeConverter кладёт «PDF» в тот же fakeStorage — как настоящий helper/.
type fakeConverter struct {
	storage  *fakeStorage
	err      error
	fileURLs []string
	n        int
}

func (c *fakeConverter) ConvertToPDF(_ context.Context, fileURL, _ string) (string, error) {
	c.fileURLs = append(c.fileURLs, fileURL)
	if c.err != nil {
		return "", c.err
	}
	c.n++
	key := fmt.Sprintf("converted/preview-%d.pdf", c.n)
	c.storage.objects[key] = true
	return key, nil
}

type fixture struct {
	svc       *template.Service
	repo      *fakeRepo
	storage   *fakeStorage
	converter *fakeConverter
}

func newFixture() fixture {
	repo, storage := newFakeRepo(), newFakeStorage()
	converter := &fakeConverter{storage: storage}
	svc := template.New(repo, storage, converter, clock.Fake{T: time.Unix(1_700_000_000, 0)}, idgen.UUIDGenerator{}, testMaxSize)
	return fixture{svc: svc, repo: repo, storage: storage, converter: converter}
}

func (f fixture) createTemplate(t *testing.T, name string, data []byte) template.TemplateView {
	t.Helper()
	view, err := f.svc.Create(context.Background(), template.CreateRequest{
		DocumentTypeID: typeID,
		Title:          "Договор аренды",
		File:           template.File{OriginalName: name, Data: data},
	})
	require.NoError(t, err)
	return view
}

func TestCreateType_NormalizesAndValidatesName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "trims spaces", input: "  Приказ  ", want: "Приказ"},
		{name: "empty", input: "   ", wantErr: template.ErrInvalidInput},
		{name: "too long", input: strings.Repeat("я", template.MaxTypeNameLength+1), wantErr: template.ErrInvalidInput},
		{name: "duplicate ignoring case", input: "договор", wantErr: template.ErrTypeNameTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			created, err := newFixture().svc.CreateType(context.Background(), tt.input)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, created.Name)
		})
	}
}

func TestDeleteType_InUse(t *testing.T) {
	f := newFixture()
	f.createTemplate(t, "lease.pdf", pdfData)

	err := f.svc.DeleteType(context.Background(), typeID)

	require.ErrorIs(t, err, template.ErrTypeInUse)
}

func TestCreate_PDFIsItsOwnPreview(t *testing.T) {
	f := newFixture()

	view := f.createTemplate(t, "Lease.PDF", pdfData)

	require.Equal(t, "application/pdf", view.Template.MimeType)
	require.Equal(t, view.Template.ObjectKey, view.Template.PreviewObjectKey)
	require.True(t, strings.HasPrefix(view.Template.ObjectKey, "templates/"))
	require.Equal(t, "https://storage.public/"+view.Template.ObjectKey, view.PreviewURL)
	require.Empty(t, f.converter.fileURLs, "PDF не конвертируется")
	require.Equal(t, []string{view.Template.ObjectKey}, f.storage.keys())
}

func TestCreate_DOCXIsConvertedForPreview(t *testing.T) {
	f := newFixture()

	view := f.createTemplate(t, "lease.docx", docxData)

	require.Equal(t, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", view.Template.MimeType)
	require.Equal(t, []string{"https://storage.internal/" + view.Template.ObjectKey}, f.converter.fileURLs)
	require.Equal(t, "converted/preview-1.pdf", view.Template.PreviewObjectKey)
	require.Equal(t, "https://storage.public/converted/preview-1.pdf", view.PreviewURL)
}

func TestCreate_RejectedWithoutLeavingObjects(t *testing.T) {
	tests := []struct {
		name    string
		req     template.CreateRequest
		convErr error
		repoErr error
		wantErr error
	}{
		{
			name:    "unsupported extension",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: "T", File: template.File{OriginalName: "a.txt", Data: []byte("text")}},
			wantErr: template.ErrUnsupportedFormat,
		},
		{
			name:    "extension does not match content",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: "T", File: template.File{OriginalName: "a.pdf", Data: docxData}},
			wantErr: template.ErrUnsupportedFormat,
		},
		{
			name:    "too large",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: "T", File: template.File{OriginalName: "a.pdf", Data: append([]byte("%PDF-"), make([]byte, testMaxSize)...)}},
			wantErr: template.ErrTooLarge,
		},
		{
			name:    "missing file",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: "T"},
			wantErr: template.ErrInvalidInput,
		},
		{
			name:    "empty title",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: " ", File: template.File{OriginalName: "a.pdf", Data: pdfData}},
			wantErr: template.ErrInvalidInput,
		},
		{
			name:    "unknown type",
			req:     template.CreateRequest{DocumentTypeID: "missing", Title: "T", File: template.File{OriginalName: "a.pdf", Data: pdfData}},
			wantErr: template.ErrTypeNotFound,
		},
		{
			name:    "conversion failed",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: "T", File: template.File{OriginalName: "a.docx", Data: docxData}},
			convErr: errors.New("helper unavailable"),
			wantErr: template.ErrConversionFailed,
		},
		{
			name:    "record not saved",
			req:     template.CreateRequest{DocumentTypeID: typeID, Title: "T", File: template.File{OriginalName: "a.docx", Data: docxData}},
			repoErr: errors.New("db down"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			f.converter.err = tt.convErr
			f.repo.writeErr = tt.repoErr

			_, err := f.svc.Create(context.Background(), tt.req)

			require.Error(t, err)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			}
			require.Empty(t, f.storage.keys())
			require.Empty(t, f.repo.templates)
		})
	}
}

func TestUpdate_WithoutFileKeepsObjects(t *testing.T) {
	f := newFixture()
	created := f.createTemplate(t, "lease.docx", docxData)
	otherType, err := f.svc.CreateType(context.Background(), "Приказ")
	require.NoError(t, err)

	updated, err := f.svc.Update(context.Background(), created.Template.ID, template.UpdateRequest{
		DocumentTypeID: otherType.ID,
		Title:          "  Новое название ",
	})

	require.NoError(t, err)
	require.Equal(t, otherType.ID, updated.Template.DocumentTypeID)
	require.Equal(t, "Новое название", updated.Template.Title)
	require.Equal(t, created.Template.ObjectKeys(), updated.Template.ObjectKeys())
	require.ElementsMatch(t, created.Template.ObjectKeys(), f.storage.keys())
}

func TestUpdate_ReplacingFileDeletesOldObjects(t *testing.T) {
	f := newFixture()
	created := f.createTemplate(t, "lease.docx", docxData)

	updated, err := f.svc.Update(context.Background(), created.Template.ID, template.UpdateRequest{
		DocumentTypeID: typeID,
		Title:          "Договор аренды",
		File:           &template.File{OriginalName: "lease-v2.pdf", Data: pdfData},
	})

	require.NoError(t, err)
	require.Equal(t, "lease-v2.pdf", updated.Template.OriginalName)
	require.Equal(t, []string{updated.Template.ObjectKey}, f.storage.keys())
}

func TestUpdate_FailedSaveKeepsOldFile(t *testing.T) {
	f := newFixture()
	created := f.createTemplate(t, "lease.pdf", pdfData)
	f.repo.writeErr = errors.New("db down")

	_, err := f.svc.Update(context.Background(), created.Template.ID, template.UpdateRequest{
		DocumentTypeID: typeID,
		Title:          "Договор аренды",
		File:           &template.File{OriginalName: "lease.docx", Data: docxData},
	})

	require.Error(t, err)
	require.Equal(t, []string{created.Template.ObjectKey}, f.storage.keys())
}

func TestUpdate_UnknownTemplate(t *testing.T) {
	_, err := newFixture().svc.Update(context.Background(), "missing", template.UpdateRequest{DocumentTypeID: typeID, Title: "T"})

	require.ErrorIs(t, err, template.ErrNotFound)
}

func TestDelete_RemovesRecordAndObjects(t *testing.T) {
	f := newFixture()
	created := f.createTemplate(t, "lease.docx", docxData)

	require.NoError(t, f.svc.Delete(context.Background(), created.Template.ID))

	require.Empty(t, f.repo.templates)
	require.Empty(t, f.storage.keys())
	require.ErrorIs(t, f.svc.Delete(context.Background(), created.Template.ID), template.ErrNotFound)
}
