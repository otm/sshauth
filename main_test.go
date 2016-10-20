package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
)

type fakeS3 struct {
	getObjectOutput s3.GetObjectOutput
}

func (f fakeS3) GetObject(i *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
	return &f.getObjectOutput, nil
}

func (f fakeS3) ListObjectsPages(i *s3.ListObjectsInput, fn func(p *s3.ListObjectsOutput, lastPage bool) (shouldContinue bool)) error {
	return nil
}

func TestReadAuthorizedKeyNewLine(t *testing.T) {
	body := "asdfghjkl\n"
	svc = &fakeS3{
		getObjectOutput: s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewBufferString(body)),
		},
	}
	keys := make(chan io.Reader)
	go readAuthorizedKey("bucket", "/a/path", keys)
	got, err := ioutil.ReadAll(<-keys)
	if err != nil {
		t.Errorf("Unable to convert to string: %v", err)
	}
	if string(got) != body {
		t.Errorf("Expected key is wrong: Got: %v, Wanted: %v", got, body)
	}

}

func TestReadAuthorizedKeyNoNewLine(t *testing.T) {
	body := "asdfghjkl"
	svc = &fakeS3{
		getObjectOutput: s3.GetObjectOutput{
			Body: ioutil.NopCloser(bytes.NewBufferString(body)),
		},
	}
	keys := make(chan io.Reader)
	go readAuthorizedKey("bucket", "/a/path", keys)
	got, err := ioutil.ReadAll(<-keys)
	if err != nil {
		t.Errorf("Unable to convert to string: %v", err)
	}
	if string(got) != body+"\n" {
		t.Errorf("Expected key is wrong: Got: %v, Wanted: %v", got, body)
	}

}
