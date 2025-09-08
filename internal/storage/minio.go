package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig MinIO配置
type MinIOConfig struct {
	Endpoint        string `yaml:"endpoint" env:"MINIO_ENDPOINT" default:"localhost:9000"`
	AccessKeyID     string `yaml:"access_key_id" env:"MINIO_ACCESS_KEY_ID" default:"minioadmin"`
	SecretAccessKey string `yaml:"secret_access_key" env:"MINIO_SECRET_ACCESS_KEY" default:"minioadmin"`
	UseSSL          bool   `yaml:"use_ssl" env:"MINIO_USE_SSL" default:"false"`
	BucketName      string `yaml:"bucket_name" env:"MINIO_BUCKET_NAME" default:"moonshot"`
	Region          string `yaml:"region" env:"MINIO_REGION" default:"us-east-1"`
}

// MinIOStorage MinIO存储实现
type MinIOStorage struct {
	client *minio.Client
	config *MinIOConfig
}

// NewMinIOStorage 创建MinIO存储
func NewMinIOStorage(config *MinIOConfig) (*MinIOStorage, error) {
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKeyID, config.SecretAccessKey, ""),
		Secure: config.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建MinIO客户端失败: %w", err)
	}

	return &MinIOStorage{
		client: client,
		config: config,
	}, nil
}

// EnsureBucket 确保存储桶存在
func (m *MinIOStorage) EnsureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.config.BucketName)
	if err != nil {
		return fmt.Errorf("检查存储桶失败: %w", err)
	}

	if !exists {
		err = m.client.MakeBucket(ctx, m.config.BucketName, minio.MakeBucketOptions{
			Region: m.config.Region,
		})
		if err != nil {
			return fmt.Errorf("创建存储桶失败: %w", err)
		}
	}

	return nil
}

// UploadFile 上传文件
func (m *MinIOStorage) UploadFile(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) error {
	_, err := m.client.PutObject(ctx, m.config.BucketName, objectName, reader, objectSize, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("上传文件失败: %w", err)
	}

	return nil
}

// DownloadFile 下载文件
func (m *MinIOStorage) DownloadFile(ctx context.Context, objectName string) (io.ReadCloser, error) {
	object, err := m.client.GetObject(ctx, m.config.BucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("下载文件失败: %w", err)
	}

	return object, nil
}

// DeleteFile 删除文件
func (m *MinIOStorage) DeleteFile(ctx context.Context, objectName string) error {
	err := m.client.RemoveObject(ctx, m.config.BucketName, objectName, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("删除文件失败: %w", err)
	}

	return nil
}

// GetFileInfo 获取文件信息
func (m *MinIOStorage) GetFileInfo(ctx context.Context, objectName string) (*FileInfo, error) {
	stat, err := m.client.StatObject(ctx, m.config.BucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("获取文件信息失败: %w", err)
	}

	return &FileInfo{
		Name:         stat.Key,
		Size:         stat.Size,
		LastModified: stat.LastModified,
		ContentType:  stat.ContentType,
		ETag:         stat.ETag,
	}, nil
}

// GeneratePresignedURL 生成预签名URL
func (m *MinIOStorage) GeneratePresignedURL(ctx context.Context, objectName string, expires time.Duration) (string, error) {
	presignedURL, err := m.client.PresignedGetObject(ctx, m.config.BucketName, objectName, expires, nil)
	if err != nil {
		return "", fmt.Errorf("生成预签名URL失败: %w", err)
	}

	return presignedURL.String(), nil
}

// ListFiles 列出文件
func (m *MinIOStorage) ListFiles(ctx context.Context, prefix string) ([]*FileInfo, error) {
	var files []*FileInfo

	objectCh := m.client.ListObjects(ctx, m.config.BucketName, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			return nil, fmt.Errorf("列出文件失败: %w", object.Err)
		}

		files = append(files, &FileInfo{
			Name:         object.Key,
			Size:         object.Size,
			LastModified: object.LastModified,
			ContentType:  object.ContentType,
			ETag:         object.ETag,
		})
	}

	return files, nil
}

// FileInfo 文件信息
type FileInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ContentType  string    `json:"content_type"`
	ETag         string    `json:"etag"`
}

// StorageInterface 存储接口
type StorageInterface interface {
	EnsureBucket(ctx context.Context) error
	UploadFile(ctx context.Context, objectName string, reader io.Reader, objectSize int64, contentType string) error
	DownloadFile(ctx context.Context, objectName string) (io.ReadCloser, error)
	DeleteFile(ctx context.Context, objectName string) error
	GetFileInfo(ctx context.Context, objectName string) (*FileInfo, error)
	GeneratePresignedURL(ctx context.Context, objectName string, expires time.Duration) (string, error)
	ListFiles(ctx context.Context, prefix string) ([]*FileInfo, error)
}