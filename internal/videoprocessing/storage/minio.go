package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	mc            *minio.Client
	presignClient *minio.Client
	videosBucket  string
	resultsBucket string
}

// NewClient wires two MinIO clients against the same bucket credentials:
// mc talks to the internal endpoint (e.g. the Docker network's "minio:9000")
// for server-side reads/writes, while presignClient signs URLs against
// publicEndpoint, the host a browser can actually resolve. They differ
// whenever the API runs behind a different network boundary than its
// clients — same signing keys, different advertised host.
func NewClient(endpoint, publicEndpoint, accessKey, secretKey, videosBucket, resultsBucket string) (*Client, error) {
	creds := credentials.NewStaticV4(accessKey, secretKey, "")

	// Region is pinned explicitly so presignClient never needs to reach
	// the server to auto-detect it — it points at publicEndpoint, which
	// isn't reachable from inside the network the API runs in.
	const region = "us-east-1"

	mc, err := minio.New(endpoint, &minio.Options{Creds: creds, Secure: false, Region: region})
	if err != nil {
		return nil, err
	}

	presignClient, err := minio.New(publicEndpoint, &minio.Options{Creds: creds, Secure: false, Region: region})
	if err != nil {
		return nil, err
	}

	return &Client{mc: mc, presignClient: presignClient, videosBucket: videosBucket, resultsBucket: resultsBucket}, nil
}

// EnsureBuckets creates the videos/results buckets if they don't already
// exist. Idempotent and safe to call from every video-api/video-worker
// replica on startup — replacing the separate createbuckets init
// container (docs/adr/0009).
func (c *Client) EnsureBuckets(ctx context.Context) error {
	for _, bucket := range []string{c.videosBucket, c.resultsBucket} {
		exists, err := c.mc.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("check bucket %q: %w", bucket, err)
		}
		if exists {
			continue
		}
		if err := c.mc.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("create bucket %q: %w", bucket, err)
		}
	}
	return nil
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
	u, err := c.presignClient.PresignedGetObject(ctx, c.resultsBucket, key, expiry, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}
