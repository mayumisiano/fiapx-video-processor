//go:build integration

package storage_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"video-processor/internal/videoprocessing/storage"
)

const fakeVideoContent = "fake video bytes for the integration test"

func TestClient_EnsureBucketsUploadDownloadAndPresign(t *testing.T) {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("MINIO_ENDPOINT not set, skipping integration test")
	}
	accessKey := os.Getenv("MINIO_ACCESS_KEY")
	secretKey := os.Getenv("MINIO_SECRET_KEY")

	client, err := storage.NewClient(endpoint, endpoint, accessKey, secretKey, "it-videos", "it-results")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := client.EnsureBuckets(ctx); err != nil {
		t.Fatalf("ensure buckets: %v", err)
	}
	// Idempotent: a second call against already-existing buckets must not fail.
	if err := client.EnsureBuckets(ctx); err != nil {
		t.Fatalf("ensure buckets (second call): %v, want nil (idempotent)", err)
	}

	srcFile := filepath.Join(t.TempDir(), "video.mp4")
	if err := os.WriteFile(srcFile, []byte(fakeVideoContent), 0o644); err != nil {
		t.Fatalf("write temp source file: %v", err)
	}

	const videoKey = "requests/it-request/video.mp4"
	if err := client.UploadVideo(ctx, videoKey, srcFile, "video/mp4"); err != nil {
		t.Fatalf("upload video: %v", err)
	}

	if err := client.StatVideo(ctx, videoKey); err != nil {
		t.Fatalf("stat video: %v", err)
	}

	destFile := filepath.Join(t.TempDir(), "downloaded.mp4")
	if err := client.DownloadVideo(ctx, videoKey, destFile); err != nil {
		t.Fatalf("download video: %v", err)
	}
	got, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(got) != fakeVideoContent {
		t.Fatalf("downloaded content = %q, want %q", got, fakeVideoContent)
	}

	const resultKey = "requests/it-request/result.zip"
	if err := client.UploadResult(ctx, resultKey, srcFile); err != nil {
		t.Fatalf("upload result: %v", err)
	}

	url, err := client.PresignedResultURL(ctx, resultKey, time.Minute)
	if err != nil {
		t.Fatalf("presigned result url: %v", err)
	}
	if url == "" {
		t.Fatal("presigned result url is empty")
	}

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET presigned url: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET presigned url status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read presigned url response body: %v", err)
	}
	if string(body) != fakeVideoContent {
		t.Fatalf("presigned url body = %q, want %q", body, fakeVideoContent)
	}
}
