# S3AccountFinder

Allows you to find the AWS account ID that owns an S3 bucket. The program performs a binary search for each digit of the account ID by iterating through possible prefixes and checking if access can be obtained with specific policies. This version includes significant performance improvements using concurrency and optimized API calls.

## Features

- **Binary Search**: Efficiently searches for each digit of the AWS account ID using binary search.
- **Parallel Execution**: Concurrently searches possible digits using goroutines to speed up the discovery process.
- **AWS IAM Role Support**: Supports assuming a specific role (`role_arn`) to check access permissions.
- **Region Caching**: Caches the S3 bucket region to reduce redundant region lookups during API calls.

## Installation

Assuming you have Go installed and configured (i.e. with $GOPATH/bin in your $PATH):

   ```bash
   go install github.com/cybercdh/S3AccountFinder@latest
   ```

## Usage

You will need an IAM role that you can assume with `ListBucket` or `GetObject` permissions on the bucket of interest.

```bash
S3AccountFinder -role_arn <role_arn> -path <s3_path>
```
Example

- `S3AccountFinder -role_arn arn:aws:iam::012345678901:role/s3-account-finder -path some-bucket`

### Parameters

- `-role_arn`: The Amazon Resource Name (ARN) of the IAM role to assume.
- `-path`: The S3 bucket or S3 bucket path (e.g., `s3://mybucket` or `s3://mybucket/mykey`) to check access against.


## Acknowledgments

This tool is inspired by the original [s3-account-search](https://github.com/WeAreCloudar/s3-account-search) project developed by [WeAreCloudar](https://github.com/WeAreCloudar). The foundational concept of searching for AWS account IDs associated with S3 buckets originates from their Python implementation.

## Performance Improvements

This version of the code includes several key optimizations, resulting in a substantial performance increase:

1. **Parallelized Digit Search**: The search for each account digit is now done concurrently using Go's goroutines. Instead of testing each digit one by one, all possible digits are checked in parallel, reducing the overall time to find the account ID.

2. **Reduced API Calls**: The binary search now batches the policy prefix checks more efficiently by dividing the digit space in half at each step, minimizing unnecessary API calls.

3. **Region Caching**: The S3 bucket region is now cached after the first lookup, so subsequent API calls do not need to fetch the bucket's region again, reducing latency.

4. **Improved Error Handling**: Instead of terminating the entire program upon an error, more graceful error handling is used, allowing retries or partial failures to be handled appropriately without crashing.

