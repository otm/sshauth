package main

import (
	"flag"
	"fmt"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/aws/awsutil"
	"github.com/awslabs/aws-sdk-go/service/s3"
)

const (
	flagFileName = "/etc/sshauth/sshauth.conf"
	debug        = "false"
)

func main() {
	flag.Parse()

	listObjects(*bucket, *key)
}

func getObject(bucket, key string) {
	svc := s3.New(nil)

	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	resp, err := svc.GetObject(params)

	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		fmt.Println("Error:", awserr.Code, awserr.Message)
	} else if err != nil {
		// A non-service error occurred.
		panic(err)
	}

	// Pretty-print the response data.
	fmt.Println(awsutil.StringValue(resp))
}

func listObjects(bucket, key string) {
	svc := s3.New(nil)

	params := &s3.ListObjectsInput{
		Bucket: aws.String(bucket), // Required
		Prefix: aws.String(key),
	}
	resp, err := svc.ListObjects(params)

	if awserr := aws.Error(err); awserr != nil {
		// A service error occurred.
		fmt.Println("Error:", awserr.Code, awserr.Message)
	} else if err != nil {
		// A non-service error occurred.
		panic(err)
	}

	// Pretty-print the response data.
	fmt.Println(awsutil.StringValue(resp))
}

func printDbg(s ...interface{}) {
	if debug == "true" {
		fmt.Print(" * ")
		fmt.Println(s...)
	}
}
