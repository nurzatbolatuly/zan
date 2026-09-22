// Package storage — S3-совместимый клиент (aws-sdk-go-v2), MinIO локально
// (BACKEND_PLAN.md §1.1). Имплементит file.Storage/voice.Storage.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// Config — параметры подключения к MinIO/S3 (backend/internal/config.Config,
// секция Stage 4). PublicEndpoint — см. doc-комментарий у
// resolvePublicEndpoint: presigned-ссылка для стороны снаружи docker-сети
// (браузер, `go test` на хосте) не может использовать внутреннее DNS-имя
// "minio", которым говорит Endpoint внутри docker-сети.
type Config struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	AccessKey      string
	SecretKey      string
	Bucket         string
}

// Client — реализация file.Storage/voice.Storage поверх MinIO/S3. Держит
// два aws s3.Client с одними и теми же учётными данными, но разными
// BaseEndpoint — internal (реальные операции + presign для helper/, тот же
// docker-сеть) и public (presign для всего, что живёт снаружи docker-сети).
// Подписи не зависят от того, каким клиентом они выпущены — MinIO проверяет
// подпись относительно Host, который реально пришёл в запросе, поэтому два
// presign-клиента с разными BaseEndpoint дают рабочие ссылки для разных
// потребителей одного и того же объекта.
type Client struct {
	api           *s3.Client
	presignInt    *s3.PresignClient
	presignPublic *s3.PresignClient
	bucket        string
}

// New строит Client и гарантирует существование бакета (idempotent —
// self-contained bootstrap, тот же принцип, что и golang-migrate на старте:
// одна команда поднимает весь стек без ручного шага "создать бакет в
// консоли MinIO", BACKEND_PLAN.md Stage 0 DoD).
func New(ctx context.Context, cfg Config) (*Client, error) {
	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(creds),
	)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	// UsePathStyle — обязателен для MinIO (виртуальный хостинг бакетов
	// вида "bucket.minio:9000" не работает без DNS-wildcard, которого в
	// docker-сети нет; для настоящего AWS S3 это тоже валидный режим,
	// просто не тот, что рекомендован по умолчанию).
	api := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})

	publicEndpoint := cfg.PublicEndpoint
	if publicEndpoint == "" {
		publicEndpoint = cfg.Endpoint
	}
	publicAPI := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(publicEndpoint)
		o.UsePathStyle = true
	})

	c := &Client{
		api:           api,
		presignInt:    s3.NewPresignClient(api),
		presignPublic: s3.NewPresignClient(publicAPI),
		bucket:        cfg.Bucket,
	}

	if err := c.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) ensureBucket(ctx context.Context) error {
	_, err := c.api.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)})
	if err == nil {
		return nil
	}

	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) || (apiErr.ErrorCode() != "NotFound" && apiErr.ErrorCode() != "NoSuchBucket") {
		return fmt.Errorf("storage: head bucket %q: %w", c.bucket, err)
	}

	if _, err := c.api.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(c.bucket)}); err != nil {
		return fmt.Errorf("storage: create bucket %q: %w", c.bucket, err)
	}
	return nil
}

// Put загружает объект под key. size — Content-Length (обязателен для
// потокового Body, иначе SigV4 не может подписать запрос заранее).
func (c *Client) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	_, err := c.api.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("storage: put %q: %w", key, err)
	}
	return nil
}

// PutBytes — Put для уже целиком собранного в память содержимого (рендер
// документа — RenderService строит файл целиком перед загрузкой, стриминг
// туда не нужен на объёме в единицы МБ).
func (c *Client) PutBytes(ctx context.Context, key string, data []byte, contentType string) error {
	return c.Put(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}

// PresignGetPublic — ссылка для стороны снаружи docker-сети (ответ клиенту
// на POST /files/upload и GET /files/{id}).
func (c *Client) PresignGetPublic(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return c.presignGet(ctx, c.presignPublic, key, expiry)
}

// PresignGetInternal — ссылка для helper/ (то же docker-сеть, что и
// backend/) — единственное правильное значение file_url в
// FilesService.Extract/SttService.Transcribe (BACKEND_PLAN.md §3.1).
func (c *Client) PresignGetInternal(ctx context.Context, key string, expiry time.Duration) (string, error) {
	return c.presignGet(ctx, c.presignInt, key, expiry)
}

func (c *Client) presignGet(ctx context.Context, presigner *s3.PresignClient, key string, expiry time.Duration) (string, error) {
	req, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("storage: presign get %q: %w", key, err)
	}
	return req.URL, nil
}

// Delete удаляет объект — используется для голосовых записей сразу после
// успешной транскрипции (zan-backend-tz-v2.md §4.7: "обработка аудио как
// есть — вне скоупа", хранить его дальше незачем).
func (c *Client) Delete(ctx context.Context, key string) error {
	_, err := c.api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("storage: delete %q: %w", key, err)
	}
	return nil
}
