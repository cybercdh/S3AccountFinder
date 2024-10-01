package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"
)

var bucketRegionCache sync.Map // Cache for storing bucket regions

func main() {
	roleArn := flag.String("role_arn", "", "ARN of the role to assume")
	path := flag.String("path", "", "s3 bucket or bucket/path to test with")
	flag.Parse()

	if *roleArn == "" || *path == "" {
		log.Fatalf("role_arn and path are required")
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("failed to load AWS configuration: %v", err)
	}

	bucket, key := toS3Args(*path)

	// Try accessing the bucket without any restrictions
	if !canAccessWithPolicy(cfg, bucket, key, *roleArn, nil) {
		fmt.Fprintf(os.Stderr, "%s cannot access %s\n", *roleArn, bucket)
		os.Exit(1)
	}

	fmt.Println("Starting search (this can take a while)")

	accountID := searchAccountID(cfg, bucket, key, *roleArn)
	if len(accountID) != 12 {
		log.Fatalf("Could not find all 12 digits of the account ID")
	} else {
		fmt.Printf("Bucket owner account ID: %s\n", accountID)
	}
}

// Performs a binary search to find the account ID
func searchAccountID(cfg aws.Config, bucket, key, roleArn string) string {
	accountID := ""
	for len(accountID) < 12 {
		nextDigit := findNextDigitConcurrently(cfg, bucket, key, roleArn, accountID)
		if nextDigit == "" {
			log.Fatalf("Could not find the next digit for account ID")
		}
		accountID += nextDigit
		fmt.Printf("Found digits so far: %s\n", accountID)
	}
	return accountID
}

// Finds the next digit concurrently using goroutines
func findNextDigitConcurrently(cfg aws.Config, bucket, key, roleArn, prefix string) string {
	possibleDigits := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	ch := make(chan string, len(possibleDigits))

	for _, digit := range possibleDigits {
		go func(digit string) {
			testPrefix := prefix + digit
			policy := getPolicy([]string{testPrefix + "*"})
			if canAccessWithPolicy(cfg, bucket, key, roleArn, policy) {
				ch <- digit
			} else {
				ch <- ""
			}
		}(digit)
	}

	for range possibleDigits {
		if nextDigit := <-ch; nextDigit != "" {
			return nextDigit
		}
	}

	return ""
}

// Constructs the policy to check for the account ID prefixes
func getPolicy(prefixes []string) map[string]interface{} {
	return map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Sid":      "AllowResourceAccount",
				"Effect":   "Allow",
				"Action":   "s3:*",
				"Resource": "*",
				"Condition": map[string]interface{}{
					"StringLike": map[string]interface{}{
						"s3:ResourceAccount": prefixes,
					},
				},
			},
		},
	}
}

// Assumes the role and applies the test policy to check access
func canAccessWithPolicy(cfg aws.Config, bucket, key, roleArn string, policy map[string]interface{}) bool {
	ctx := context.TODO()

	// Assume the role using stscreds
	stsSvc := sts.NewFromConfig(cfg)
	creds := stscreds.NewAssumeRoleProvider(stsSvc, roleArn, func(opt *stscreds.AssumeRoleOptions) {
		if policy != nil {
			policyString := marshalPolicy(policy)
			opt.Policy = aws.String(policyString)
		}
	})

	// Check bucket region cache before querying
	bucketRegion, found := bucketRegionCache.Load(bucket)
	if !found {
		// Create S3 client with assumed role credentials and default region
		s3Svc := s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.Credentials = aws.NewCredentialsCache(creds)
			o.Region = "us-east-1" // Default region for S3
		})

		// Get the bucket region
		region, err := manager.GetBucketRegion(ctx, s3Svc, bucket)
		if err != nil {
			log.Fatalf("Failed to get bucket region: %v", err)
		}
		bucketRegionCache.Store(bucket, region)
		bucketRegion = region
	}

	// Create a new S3 client with the correct region
	s3Svc := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.Credentials = aws.NewCredentialsCache(creds)
		o.Region = bucketRegion.(string)
	})

	var result bool
	if key != "" {
		// Try HeadObject
		_, err := s3Svc.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) {
				errorCode := apiErr.ErrorCode()
				if errorCode == "403" || errorCode == "AccessDenied" || errorCode == "Forbidden" {
					result = false
				} else if errorCode == "404" || errorCode == "NotFound" {
					result = true
				} else {
					log.Fatalf("Unexpected error code %s: %v", errorCode, err)
				}
			} else {
				log.Fatalf("Unexpected error: %v", err)
			}
		} else {
			result = true
		}
	} else {
		// Try HeadBucket
		_, err := s3Svc.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			var apiErr smithy.APIError
			if errors.As(err, &apiErr) {
				errorCode := apiErr.ErrorCode()
				if errorCode == "403" || errorCode == "AccessDenied" || errorCode == "Forbidden" {
					result = false
				} else if errorCode == "404" || errorCode == "NotFound" {
					result = true
				} else {
					log.Fatalf("Unexpected error code %s: %v", errorCode, err)
				}
			} else {
				log.Fatalf("Unexpected error: %v", err)
			}
		} else {
			result = true
		}
	}

	return result
}

// Converts the path to bucket and key
func toS3Args(path string) (string, string) {
	if strings.HasPrefix(path, "s3://") {
		path = path[5:]
	}
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

// Marshals the policy map to a JSON string
func marshalPolicy(policy map[string]interface{}) string {
	policyBytes, err := json.Marshal(policy)
	if err != nil {
		log.Fatalf("Failed to marshal policy: %v", err)
	}
	return string(policyBytes)
}
