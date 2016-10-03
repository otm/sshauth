package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"log/syslog"
	"os"
	"os/signal"
	"path"
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

	logheader = "command=\"%s %s %s %s\" "
)

var (
	bucket            = flag.String("bucket", "", "S3 bucket `name`")
	key               = flag.String("key", "", "S3 bucket `prefix`")
	region            = flag.String("region", "", "AWS `region`, eg. eu-west-1")
	authlog           = flag.String("authlog", "", "Set `path` to sshlogger script")
	syslogEnabled     = flag.Bool("syslog", false, "Enable logging via syslog")
	printSSHLogger    = flag.Bool("sshlogger", false, "Print sshlogger script to stdout")
	enableDebugOutput = flag.Bool("debug", false, "Enable debug output")

	debug = log.New(ioutil.Discard, " * ", 0)

	svc = s3.New(nil)
)

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: sshauth [OPTIONS] <username>\n\nOptions:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\nThe final S3 URL will be: bucket/prefix/username")
}

func usageError(error string) {
	fmt.Fprintln(os.Stderr, error)
	usage()
	os.Exit(1)
}

func init() {
	flag.Usage = usage
}

func main() {
	log.SetFlags(0)
	readDefaultFlagFile()
	flag.Parse()

	// setup normal logger
	if *syslogEnabled {
		var err error
		syslogWriter, err := syslog.New(syslog.LOG_NOTICE, "sshauth")
		if err != nil {
			log.Fatalf("unable to initialize sysloger: %v", err)
		}
		log.SetOutput(syslogWriter)
	}

	//
	if *enableDebugOutput {
		debug.SetOutput(os.Stderr)

		if *syslogEnabled {
			syslogWriter, err := syslog.New(syslog.LOG_INFO, "sshauth")
			if err != nil {
				log.Fatalf("unable to setup syslogger: %v", err)
			}
			debug.SetOutput(syslogWriter)
		}
	}

	if *printSSHLogger {
		fmt.Println(sshlogger)
		os.Exit(0)
	}

	if *bucket == "" {
		usageError("Error: S3 bucket is required")
	}

	if flag.NArg() != 1 {
		usageError("Error: Username is required")
	}

	if *region != "" {
		defaults.DefaultConfig = defaults.DefaultConfig.WithRegion(*region).WithMaxRetries(10)
		debug.Printf("Setting region: %s", *region)
		svc = s3.New(nil)
	}

	user := flag.Arg(0)

	go listenOnSigpipe()

	printAuthorizedKeys(*bucket, *key, user)
}

func listenOnSigpipe() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGPIPE)
	<-c
	debug.Println("Got SIGPIPE signal")
}

// readAuthorizedKey reads the authorized keys from S3
func readAuthorizedKey(bucket, key string, authorizedKeys chan io.Reader) {
	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	resp, err := svc.GetObject(params)

	if err != nil {
		switch e := err.(type) {
		case awserr.Error:
			debug.Printf("unable to get authorized key from S3: %s: %s", e.Code(), e.Message())
		default:
			debug.Printf("unable to get authorized key from S3: %v", e)
		}
		authorizedKeys <- bytes.NewReader([]byte{})
		return
	}

	if *authlog != "" {
		outbuf := bytes.NewBufferString(fmt.Sprintf(logheader, *authlog, path.Base(key), bucket, key))
		outbuf.ReadFrom(resp.Body)
		authorizedKeys <- outbuf
		return
	}

	authorizedKeys <- resp.Body
}

// printAuthorizedKeys for specified bucket, prefix (key) and user
// the used path will be bucket/prefix/user/*
func printAuthorizedKeys(bucket, authorizedKeysPath, user string) {
	authorizedKeys := make(chan io.Reader, 5)

	authorizedKeysPath = path.Join(authorizedKeysPath, user)
	debug.Printf("listing authorized keys in bucket: %s, path: %s", bucket, authorizedKeysPath)

	params := &s3.ListObjectsInput{
		Bucket: aws.String(bucket), // Required
		Prefix: aws.String(authorizedKeysPath),
	}

	err := svc.ListObjectsPages(params, func(resp *s3.ListObjectsOutput, lastPage bool) (shouldContinue bool) {
		for _, content := range resp.Contents {
			// If it's a root key skip reading it
			if *content.Key == authorizedKeysPath {
				authorizedKeys <- bytes.NewReader([]byte{})
				continue
			}
			go readAuthorizedKey(bucket, *content.Key, authorizedKeys)
		}

		for range resp.Contents {
			_, err := io.Copy(os.Stdout, <-authorizedKeys)
			if err != nil {
				if err == syscall.EPIPE {
					// Expected error
					return false
				}
				log.Println("Unable to copy authorized key to stdout:", err)
			}
		}

		return !lastPage
	})

	if err != nil {
		switch e := err.(type) {
		case awserr.Error:
			log.Fatalf("unable to list authorized keys: %s, message: %s", e.Code(), e.Message())
		default:
			log.Fatal("Error:", e)
		}
	}
}

// readDefaultFlagFile reads the default flag file, see readFlagFile
func readDefaultFlagFile() {
	readFlagFile(flagFileName)
}

// readFlagFile will read a file containing command line flags.
// These flags will be added before flags on the command line, therfore
// those flags will override. Note, if flags are read into an array they
// will not be overridden, but appended.
func readFlagFile(flagFileName string) {
	flagFile, err := os.Open(flagFileName)
	if err != nil {
		dir, _ := os.Getwd()
		debug.Println("Unable to open file: ", flagFileName, ", In folder:", dir)
		return
	}
	defer flagFile.Close()

	debug.Println("Reading flag file:", flagFileName)

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
