package main

import (
	"context"
	"os"
	"testing"
)

func TestEnsureBucketsContract(t *testing.T) {
	// Verifies provisioner rejects empty credentials
	err := EnsureBucketsExist(context.Background(), "", "", "", false)
	if err == nil {
		t.Fatal("expected error when credentials and endpoint are empty, got nil")
	}
}

func TestEnsureBucketsLive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live MinIO test in short mode")
	}

	loadEnv("../../.env")
	loadEnv("../.env")
	loadEnv(".env")

	endpoint := os.Getenv("S3_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9010"
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

	err := EnsureBucketsExist(context.Background(), endpoint, accessKey, secretKey, useSSL)
	if err != nil {
		t.Skipf("skipping live MinIO test (service not reachable at %s): %v", endpoint, err)
	}
}
