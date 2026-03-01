package storage

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	appconfig "github.com/redoubtapp/redoubt-api/internal/config"
)

// ErrObjectNotFound is returned when an S3 object does not exist.
var ErrObjectNotFound = errors.New("object not found")

// S3Client wraps the AWS S3 client with additional functionality.
type S3Client struct {
	client *s3.Client
	bucket string
}

// NewS3Client creates a new S3 client configured for the given storage settings.
func NewS3Client(ctx context.Context, cfg appconfig.StorageConfig) (*S3Client, error) {
	// Build AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKey,
			cfg.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with path-style addressing for compatibility
	// Use BaseEndpoint for custom S3-compatible endpoints (LocalStack, MinIO, GCS, etc.)
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		// Disable automatic checksum calculation for compatibility with
		// S3-compatible providers (GCS, MinIO, etc.) that don't support
		// the default CRC32 checksum headers added in AWS SDK v1.73.0+.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
	})

	return &S3Client{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// EnsureBucket creates the bucket if it doesn't exist.
// In production, the bucket should already exist; this is primarily for dev/first-time setup.
func (c *S3Client) EnsureBucket(ctx context.Context) error {
	// Check if bucket exists
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(c.bucket),
	})
	if err == nil {
		return nil // Bucket exists
	}

	// If we get a 403, the bucket likely exists but we lack ListBucket/HeadBucket permissions.
	// This is common with GCS and restricted IAM policies. Skip creation.
	var respErr interface{ HTTPStatusCode() int }
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == 403 {
		return nil
	}

	// Try to create the bucket (useful for dev environments like LocalStack/MinIO)
	_, createErr := c.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(c.bucket),
	})
	if createErr != nil {
		// If creation fails because it already exists, that's fine
		var bae *types.BucketAlreadyExists
		var baob *types.BucketAlreadyOwnedByYou
		if errors.As(createErr, &bae) || errors.As(createErr, &baob) {
			return nil
		}
		return fmt.Errorf("failed to create bucket: %w", createErr)
	}

	return nil
}

// Upload uploads data to S3 with the given key.
func (c *S3Client) Upload(ctx context.Context, key string, data io.Reader, contentType string, size int64) error {
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          data,
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(size),
	})
	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}

	return nil
}

// Download downloads data from S3 with the given key.
func (c *S3Client) Download(ctx context.Context, key string) (io.ReadCloser, string, error) {
	output, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check for NoSuchKey error
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, "", ErrObjectNotFound
		}
		return nil, "", fmt.Errorf("failed to download object: %w", err)
	}

	contentType := ""
	if output.ContentType != nil {
		contentType = *output.ContentType
	}

	return output.Body, contentType, nil
}

// Delete deletes an object from S3.
func (c *S3Client) Delete(ctx context.Context, key string) error {
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// Exists checks if an object exists in S3.
func (c *S3Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// Check if it's a not found error
		return false, nil
	}

	return true, nil
}
