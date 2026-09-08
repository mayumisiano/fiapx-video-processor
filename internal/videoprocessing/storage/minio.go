package storage

import (
	"context"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc            *minio.Client
	videosBucket  string
	resultsBucket string
}

func NewClient(endpoint, accessKey, secretKey, videosBucket, resultsBucket string) (*Client, error) {
	mc, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	if err != nil {
		return nil, err
	}
	return &Client{mc: mc, videosBucket: videosBucket, resultsBucket: resultsBucket}, nil
}

func (c *Client) UploadVideo(ctx context.Context, key, filePath, contentType string) error {
	_, err := c.mc.FPutObject(ctx, c.videosBucket, key, filePath, minio.PutObjectOptions{ContentType: contentType})
	return err
}

func (c *Client) StatVideo(ctx context.Context, key string) error {
	_, err := c.mc.StatObject(ctx, c.videosBucket, key, minio.StatObjectOptions{})
	return err
}

func (c *Client) DownloadVideo(ctx context.Context, key, destPath string) error {
	return c.mc.FGetObject(ctx, c.videosBucket, key, destPath, minio.GetObjectOptions{})
}

func (c *Client) UploadResult(ctx context.Context, key, filePath string) error {
	_, err := c.mc.FPutObject(ctx, c.resultsBucket, key, filePath, minio.PutObjectOptions{ContentType: "application/zip"})
	return err
}

func (c *Client) PresignedResultURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.resultsBucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
