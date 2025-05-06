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

	// if endpoint != "" {
	// 	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
	// 		if service == s3.ServiceID {
	// 			return aws.Endpoint{
	// 				URL:               endpoint,
	// 				SigningRegion:     region,
	// 				HostnameImmutable: true,
	// 			}, nil
	// 		}
	// 		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	// 	})
	// 	if strings.HasPrefix(endpoint, "http://") {
	// 		opts = append(opts,
	// 			config.WithHTTPClient(&http.Client{
	// 				Transport: &http.Transport{
	// 					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	// 				},
	// 			}),
	// 		)
	// 	}
	// 	opts = append(opts, config.WithEndpointResolverWithOptions(customResolver))
	// }

	cfg, err := config.LoadDefaultConfig(context.TODO(), opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &S3{
		Client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true // used for MinIO
			o.BaseEndpoint = aws.String(endpoint)
			o.Region = region

			if strings.HasPrefix(endpoint, "http://") {
				o.EndpointOptions.DisableHTTPS = true
				o.RequestChecksumCalculation = aws.RequestChecksumCalculationUnset
				o.ResponseChecksumValidation = aws.ResponseChecksumValidationUnset
				o.UsePathStyle = true

			}
		}),
		Config:   &cfg,
		Endpoint: endpoint,
	}, nil
}
