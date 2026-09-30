package main

import (
	"errors"
	"testing"

	"github.com/aws/smithy-go"
)

func TestToS3Args(t *testing.T) {
	cases := []struct {
		in         string
		wantBucket string
		wantKey    string
	}{
		{"s3://my-bucket", "my-bucket", ""},
		{"s3://my-bucket/path/to/key", "my-bucket", "path/to/key"},
		{"my-bucket", "my-bucket", ""},
		{"my-bucket/key", "my-bucket", "key"},
		{"s3://my-bucket/", "my-bucket", ""},
	}
	for _, c := range cases {
		b, k := toS3Args(c.in)
		if b != c.wantBucket || k != c.wantKey {
			t.Errorf("toS3Args(%q) = (%q, %q), want (%q, %q)", c.in, b, k, c.wantBucket, c.wantKey)
		}
	}
}

type apiErr struct{ code string }

func (e apiErr) Error() string                 { return e.code }
func (e apiErr) ErrorCode() string             { return e.code }
func (e apiErr) ErrorMessage() string          { return e.code }
func (e apiErr) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func TestInterpretHeadErr(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		wantOK  bool
		wantErr bool
	}{
		{"nil means accessible", nil, true, false},
		{"403 denied", apiErr{"403"}, false, false},
		{"AccessDenied", apiErr{"AccessDenied"}, false, false},
		{"Forbidden", apiErr{"Forbidden"}, false, false},
		{"404 resolves under policy", apiErr{"404"}, true, false},
		{"NotFound resolves under policy", apiErr{"NotFound"}, true, false},
		{"unexpected api code is an error", apiErr{"Throttling"}, false, true},
		{"non-api error surfaces", errors.New("dial timeout"), false, true},
	}
	for _, c := range cases {
		ok, err := interpretHeadErr(c.err)
		if ok != c.wantOK || (err != nil) != c.wantErr {
			t.Errorf("%s: interpretHeadErr = (%v, %v), want ok=%v err=%v", c.name, ok, err, c.wantOK, c.wantErr)
		}
	}
}
