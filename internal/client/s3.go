// internal/client/s3.go
package client

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3 struct {
	Client   *s3.Client
	Config   *aws.Config
	Endpoint string
}

func NewS3(endpoint, region, accessKey, secretKey string) (*S3, error) {
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if accessKey != "" && secretKey != "" {
		creds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
		opts = append(opts, config.WithCredentialsProvider(creds))
	}

	cfg, err := config.LoadDefaultConfig(context.TODO(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &S3{
		Client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true // used for MinIO
			o.BaseEndpoint = aws.String(endpoint)
			o.Region = region

			// Disable checksums for HTTP endpoints, Storj, and DigitalOcean
			if strings.HasPrefix(endpoint, "http://") ||
				strings.Contains(endpoint, "storjshare.io") ||
				strings.Contains(endpoint, "digitaloceanspaces.com") {
				o.EndpointOptions.DisableHTTPS = strings.HasPrefix(endpoint, "http://")
				o.RequestChecksumCalculation = aws.RequestChecksumCalculationUnset
				o.ResponseChecksumValidation = aws.ResponseChecksumValidationUnset
			}
		}),
		Config:   &cfg,
		Endpoint: endpoint,
	}, nil
}
