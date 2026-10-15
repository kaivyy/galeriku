package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

const (
	BucketMasterVault = "galeriku-master-vault"
	BucketPublicMedia = "galeriku-public-media"
)

func loadEnv(filepath string) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}

func EnsureBucketsExist(ctx context.Context, endpoint, accessKey, secretKey string, useSSL bool) error {
	if endpoint == "" || accessKey == "" || secretKey == "" {
		return errors.New("endpoint, accessKey, and secretKey must not be empty")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize minio client: %w", err)
	}

	buckets := []string{BucketMasterVault, BucketPublicMedia}
	for _, bucket := range buckets {
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return fmt.Errorf("failed checking bucket %s: %w", bucket, err)
		}
		if !exists {
			err = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
			if err != nil {
				return fmt.Errorf("failed creating bucket %s: %w", bucket, err)
			}
			log.Printf("[PROVISIONER] Created S3 bucket: %s", bucket)
		} else {
			log.Printf("[PROVISIONER] S3 bucket already exists: %s", bucket)
		}
	}

	// Set public read policy on galeriku-public-media
	publicPolicy := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": ["*"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::%s/*"]
			}
		]
	}`, BucketPublicMedia)

	err = client.SetBucketPolicy(ctx, BucketPublicMedia, publicPolicy)
	if err != nil {
		return fmt.Errorf("failed to set public policy on %s: %w", BucketPublicMedia, err)
	}
	log.Printf("[PROVISIONER] Applied public-read policy to %s", BucketPublicMedia)

	return nil
}

func main() {
	loadEnv("../.env")
	loadEnv(".env")

	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	accessKey := os.Getenv("MINIO_ROOT_USER")
	if accessKey == "" {
		accessKey = "galeriku_admin"
	}
	secretKey := os.Getenv("MINIO_ROOT_PASSWORD")
	if secretKey == "" {
		secretKey = "galeriku_minio_secret"
	}

	useSSL := os.Getenv("S3_USE_SSL") == "true"

	log.Printf("[PROVISIONER] Connecting to MinIO at %s...", endpoint)
	if err := EnsureBucketsExist(context.Background(), endpoint, accessKey, secretKey, useSSL); err != nil {
		log.Fatalf("[PROVISIONER] Failed: %v", err)
	}
	log.Println("[PROVISIONER] S3 Buckets provisioned successfully.")
}
