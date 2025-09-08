package storage

import (
	"context"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/freedkr/moonshot/internal/config"
)

type Client interface {
	UploadFile(objectName string, reader io.Reader, size int64, contentType string) error
	DownloadFile(objectName string) (io.ReadCloser, error)
	DeleteFile(objectName string) error
	GetDownloadURL(objectName string, duration time.Duration) (string, error)
}

type minioClient struct {
	client     *minio.Client
	bucketName string
}

func NewMinIOClient(cfg *config.Config) (Client, error) {
	// 创建MinIO客户端
	client, err := minio.New(cfg.Storage.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.Storage.AccessKeyID, cfg.Storage.SecretAccessKey, ""),
		Secure: cfg.Storage.UseSSL,
	})
	if err != nil {
		return nil, err
	}

	mc := &minioClient{
		client:     client,
		bucketName: cfg.Storage.BucketName,
	}

	// 确保bucket存在
	err = mc.ensureBucketExists()
	if err != nil {
		return nil, err
	}

	return mc, nil
}

func (c *minioClient) ensureBucketExists() error {
	ctx := context.Background()

	exists, err := c.client.BucketExists(ctx, c.bucketName)
	if err != nil {
		return err
	}

	if !exists {
		err = c.client.MakeBucket(ctx, c.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *minioClient) UploadFile(objectName string, reader io.Reader, size int64, contentType string) error {
	ctx := context.Background()

	_, err := c.client.PutObject(ctx, c.bucketName, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})

	return err
}

func (c *minioClient) DownloadFile(objectName string) (io.ReadCloser, error) {
	ctx := context.Background()

	object, err := c.client.GetObject(ctx, c.bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}

	return object, nil
}

func (c *minioClient) DeleteFile(objectName string) error {
	ctx := context.Background()

	err := c.client.RemoveObject(ctx, c.bucketName, objectName, minio.RemoveObjectOptions{})
	return err
}

func (c *minioClient) GetDownloadURL(objectName string, duration time.Duration) (string, error) {
	ctx := context.Background()

	url, err := c.client.PresignedGetObject(ctx, c.bucketName, objectName, duration, nil)
	if err != nil {
		return "", err
	}

	return url.String(), nil
}
