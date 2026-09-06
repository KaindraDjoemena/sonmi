package api

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var S3Client *s3.Client

func InitS3Client() error {
	region := os.Getenv("AWS_REGION")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	if region == "" || accessKey == "" || secretKey == "" {
		return fmt.Errorf("missing required AWS environment variables")
	}

	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return fmt.Errorf("unable to load AWS SDK config: %v", err)
	}

	S3Client = s3.NewFromConfig(cfg)
	return nil
}

func UploadDailyPhoto(ctx context.Context, fileKey string, body io.Reader) (string, error) {
	bucket := os.Getenv("S3_BUCKET_NAME")
	if bucket == "" {
		return "", fmt.Errorf("missing S3_BUCKET_NAME environment variable")
	}

	if S3Client == nil {
		return "", fmt.Errorf("S3Client is not initialized")
	}

	_, err := S3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &fileKey,
		Body:   body,
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload image to S3: %v", err)
	}

	// https://{bucket}.s3.{region}.amazonaws.com/{key}
	region := os.Getenv("AWS_REGION")
	objectURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", bucket, region, fileKey)
	return objectURL, nil
}

// PresignDailyPhoto returns a time-limited GET URL for the S3 object behind
// imgURL (as stored in daily_photos.img_url / journal_entries.img_url), so the
// public site can load the image without the bucket being public. SigV4 with
// static credentials caps ttl at 7 days; getJournals regenerates these on every
// request and the devlog's ISR refetches daily, so served URLs stay well inside
// that window.
func PresignDailyPhoto(ctx context.Context, imgURL string, ttl time.Duration) (string, error) {
	bucket := os.Getenv("S3_BUCKET_NAME")
	if bucket == "" {
		return "", fmt.Errorf("missing S3_BUCKET_NAME environment variable")
	}
	if S3Client == nil {
		return "", fmt.Errorf("S3Client is not initialized")
	}

	u, err := url.Parse(imgURL)
	if err != nil {
		return "", fmt.Errorf("invalid image URL: %v", err)
	}
	key := strings.TrimPrefix(u.Path, "/")

	req, err := s3.NewPresignClient(S3Client).PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("failed to presign image URL: %v", err)
	}
	return req.URL, nil
}

func FetchImageAsBase64(ctx context.Context, imgUrl string) ([]byte, error) {
	bucket := os.Getenv("S3_BUCKET_NAME")
	if bucket == "" {
		return nil, fmt.Errorf("missing S3_BUCKET_NAME environment variable")
	}

	if S3Client == nil {
		return nil, fmt.Errorf("S3Client is not initialized")
	}

	u, err := url.Parse(imgUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid image URL: %v", err)
	}
	key := strings.TrimPrefix(u.Path, "/")

	result, err := S3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image from S3: %v", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image body: %v", err)
	}

	return data, nil
}
