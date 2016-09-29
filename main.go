package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/defaults"
	"github.com/aws/aws-sdk-go/service/s3"
)

//go:generate go run tools/include.go

const (
	// flagFileName is the configuration file used for sshauth
	flagFileName = "/etc/sshauth/sshauth.conf"

	// debug ("true"/"false") controls debug output
	debug = "false"

	logheader = "command=\"%s %s %s %s\" "
)

var (
	// name of s3 bucket
	bucket = flag.String("bucket", "", "S3 Bucket name")

	// name of s3 key prefix
	key = flag.String("key", "", "S3 bucket key")

	// aws region to use
	region = flag.String("region", "", "AWS Region")

	authlog = flag.String("authlog", "", "Path to sshlogger script")

	printSSHLog = flag.Bool("sshlogger", false, "Output sshlogger script")

	// username to authenticate
	user = ""

	svc = s3.New(nil)

	usage = `Usage: sshauth [options] username

Options:
 -bucket             S3 bucket name
 -key                S3 key prefix in bucket
 -region             AWS Region
 -authlog            Log key file with syslog
 -sshlogger          Install sshlogger for sysloging

The final S3 url will be: bucket/prefix/username
`
)

func init() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
}

func main() {
	readDefaultFlagFile()
	flag.Parse()

	if *printSSHLog {
		fmt.Println(sshlogger)
		os.Exit(0)
	}

	if *bucket == "" {
		fmt.Println("S3 bucket is required.")
		flag.Usage()
		os.Exit(1)
	}

	if flag.NArg() != 1 {
		fmt.Println("Username is required")
		flag.Usage()
		os.Exit(1)
	}

	if *region != "" {
		defaults.DefaultConfig = defaults.DefaultConfig.WithRegion(*region).WithMaxRetries(10)
		printDbg("Setting region:", *region)
		svc = s3.New(nil)
	}

	user = flag.Arg(0)

	go listenOnSigpipe()

	printAuthorizedKeys(*bucket, *key, user)
}

func listenOnSigpipe() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGPIPE)
	<-c
	printDbg("Got SIGPIPE signal")
}

// readAuthorizedKey reads the authorized keys from S3
func readAuthorizedKey(bucket, key string, r chan io.Reader) {
	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	resp, err := svc.GetObject(params)

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			r <- bytes.NewReader([]byte{})
			printDbgf("AWS Error(1): Code: %s, Message: %s", awsErr.Code(), awsErr.Message())
			return
		}
		r <- bytes.NewReader([]byte{})
		printDbg("Error:", err)
	}

	if *authlog != "" {
		outbuf := bytes.NewBufferString(fmt.Sprintf(logheader, *authlog, path.Base(key), bucket, key))
		outbuf.ReadFrom(resp.Body)
		r <- outbuf
		return
	}

	r <- resp.Body
}

// printAuthorizedKeys for specified bucket, prefix (key) and user
// the used path will be bucket/prefix/user/*
func printAuthorizedKeys(bucket, key, user string) {
	keys := make(chan io.Reader, 5)

	key = strings.TrimSuffix(key, "/") + "/" + user + "/"

	params := &s3.ListObjectsInput{
		Bucket: aws.String(bucket), // Required
		Prefix: aws.String(key),
	}

	err := svc.ListObjectsPages(params, func(resp *s3.ListObjectsOutput, lastPage bool) (shouldContinue bool) {
		for _, content := range resp.Contents {
			// If it's a root key skip reading it
			if *content.Key == key {
				keys <- bytes.NewReader([]byte{})
				continue
			}
			go readAuthorizedKey(bucket, *content.Key, keys)
		}

		for range resp.Contents {
			_, err := io.Copy(os.Stdout, <-keys)
			if err != nil {
				if err == syscall.EPIPE {
					// Expected error
					return false
				}
				log.Println("Unable to copy to stdout:", err)
			}
		}

		return !lastPage
	})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			log.Fatalf("AWS Error(2): Code: %s, Message: %s", awsErr.Code(), awsErr.Message())
		}
		log.Fatal("Error:", err)
	}
}

// readDefaultFlagFile reads the default flag file, see readFlagFile
func readDefaultFlagFile() {
	readFlagFile(flagFileName)
}

// readFlagFile will read a file containing command line flags.
// These flags will be added before flags on the command line, therefor
// those flags will override. Note, if flags are read into an array they
// will not be overridden, but appended.
func readFlagFile(flagFileName string) {
	flagFile, err := os.Open(flagFileName)
	if err != nil {
		dir, _ := os.Getwd()
		printDbgf("Unable to open file: %s, In folder: %s", flagFileName, dir)
		return
	}
	defer flagFile.Close()

	printDbg("Reading flag file:", flagFileName)

	var newArgs []string
	newArgs = append(newArgs, os.Args[0])

	// Read arguments from file
	scanner := bufio.NewScanner(flagFile)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		newArgs = append(newArgs, scanner.Text())
	}

	// Add arguments from command line after
	for i := 1; i < len(os.Args); i++ {
		newArgs = append(newArgs, os.Args[i])
	}

	os.Args = newArgs
}

// printDbgf formats string and call printDbg
func printDbgf(s string, p ...interface{}) {
	printDbg(fmt.Sprintf(s, p...))
}

// printDbg prints debug messages
func printDbg(s ...interface{}) {
	if debug == "true" {
		fmt.Print(" * ")
		fmt.Println(s...)
	}
}
