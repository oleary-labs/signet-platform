// Package storage handles asset uploads — app logos, operator branding, and
// any document an organization attaches to its account.
//
// In production objects live in a private S3-compatible bucket and are served
// through short-lived presigned GETs, so the bucket is never public and no
// credential ever reaches the browser. In development the same interface is
// backed by the local filesystem, so a developer needs no S3 to run the stack.
package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/oleary-labs/signet-platform/backend/internal/config"
)

// Store wraps an S3-compatible client for presigned uploads and reads.
type Store struct {
	client    *s3.Client
	presign   *s3.PresignClient
	bucket    string
	publicURL string
}

// New builds a Store from config. Works against AWS S3 or MinIO (path-style).
func New(ctx context.Context, c *config.Config) (*Store, error) {
	opts := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(c.S3Region)}
	if c.S3AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(c.S3AccessKey, c.S3SecretKey, ""),
		))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if c.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(c.S3Endpoint)
		}
		o.UsePathStyle = c.S3ForcePathStyle
	})

	return &Store{
		client:    client,
		presign:   s3.NewPresignClient(client),
		bucket:    c.S3Bucket,
		publicURL: strings.TrimRight(c.S3PublicURL, "/"),
	}, nil
}

// PresignUpload returns a PUT URL the client uploads directly to, plus the
// public URL the stored object will be served from.
func (s *Store) PresignUpload(ctx context.Context, key, contentType string) (uploadURL, publicURL string, err error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", "", err
	}
	return req.URL, fmt.Sprintf("%s/%s", s.publicURL, key), nil
}

// PresignGet returns a short-lived signed URL for a single object. The media
// endpoint redirects to it, so assets serve without making the bucket public.
func (s *Store) PresignGet(ctx context.Context, key string) (string, error) {
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(30*time.Minute))
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

// Delete removes an object. Used when an org deletes an asset it uploaded.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// Ping verifies the bucket is reachable. A HeadBucket is the cheapest call
// that proves credentials and network at once, which is what readiness needs.
func (s *Store) Ping(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	return err
}

// allowedUploadTypes is the set of content types the platform will presign.
// Uploads are rendered back into the console and into developers' own login
// modals, so anything script-bearing (SVG in particular) stays off the list.
var allowedUploadTypes = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/webp":      ".webp",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
	"text/csv":        ".csv",
}

// ValidateContentType reports the canonical extension for an accepted upload
// type, or an error naming what is allowed.
func ValidateContentType(contentType string) (string, error) {
	base, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", fmt.Errorf("unreadable content type %q", contentType)
	}
	ext, ok := allowedUploadTypes[base]
	if !ok {
		allowed := make([]string, 0, len(allowedUploadTypes))
		for t := range allowedUploadTypes {
			allowed = append(allowed, t)
		}
		return "", fmt.Errorf("content type %q is not accepted (allowed: %s)", base, strings.Join(allowed, ", "))
	}
	return ext, nil
}

// NewObjectKey builds an unguessable storage key under a scope prefix. The
// randomness is what lets the local-dev PUT endpoint stay unauthenticated and
// the production bucket stay private without per-object ACLs.
func NewObjectKey(scope, ext string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate object key: %w", err)
	}
	scope = strings.Trim(path.Clean("/"+scope), "/")
	if scope == "" || scope == "." {
		scope = "misc"
	}
	return fmt.Sprintf("%s/%s%s", scope, hex.EncodeToString(buf[:]), ext), nil
}
