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
	syslogTag    = "sshauth"
	logheader    = "command=\"%s %s %s %s\" "
)

type s3er interface {
	GetObject(*s3.GetObjectInput) (*s3.GetObjectOutput, error)
	ListObjectsPages(*s3.ListObjectsInput, func(p *s3.ListObjectsOutput, lastPage bool) (shouldContinue bool)) error
}

var (
	bucket         = flag.String("bucket", "", "S3 bucket `name`")
	key            = flag.String("key", "", "S3 bucket `prefix`")
	region         = flag.String("region", "", "AWS `region`, eg. eu-west-1")
	authlog        = flag.String("authlog", "", "Set `path` to sshlogger script")
	syslogEnabled  = flag.Bool("syslog", false, "Enable logging via syslog")
	printSSHLogger = flag.Bool("sshlogger", false, "Print sshlogger script to stdout and exit")
	enableLogging  = flag.Bool("logging", true, "Set to false to disable logging")
	enableDebugLog = flag.Bool("debug", false, "Enable debug output")

	info  = log.New(ioutil.Discard, "", 0)
	debug = log.New(ioutil.Discard, " * ", 0)

	svc s3er
)

func usage() {
	fmt.Fprintln(os.Stderr,
		`Usage: sshauth [OPTIONS] -bucket <name> [-key <prefix>] <username>
Read authorized keys from S3 to be used with AuthorizedKeysCommand in sshd
`)

	flag.PrintDefaults()

	fmt.Fprintln(os.Stderr, `
Note: The final S3 path will be: s3://bucket/key/username

CONFIGURATION
Default configuration is done by defining flags in /etc/sshauth/sshauth.conf
That is, in the same way as done on the command line.

AUTHLOG
When the authlog feature is enabled sshauth will inject a command option for
each authorized key. The command will be the one specified by the -authlog flag.
The command is provided three commmand line parameters: ´user´, ´bucket´, and
´key´. In addition, the command originally supplied by the client is available
in the SSH_ORIGINAL_COMMAND environment variable.

Create sshlogger.sh
´sshauth -sshlogger > /usr/local/bin/sshlogger.sh´

Running sshauth with authlog enabled
´sshauth -bucket myBucket -authlog /usr/local/bin/sshlogger.sh myUser´
`)
}

func usageError(error string) {
	fmt.Fprintln(os.Stderr, error)
	usage()
	os.Exit(1)
}

func main() {

	// Disable timestamp on log messages
	info.SetFlags(0)

	// Read flag file with default configuration
	readDefaultFlagFile()

	flag.Usage = usage
	flag.Parse()

	// setup normal logger
	if *enableLogging {
		info.SetOutput(os.Stderr)

		if *syslogEnabled {
			mustEnableSyslog(info, syslog.LOG_NOTICE, syslogTag)
		}
	}

	// setup debug logging
	if *enableLogging && *enableDebugLog {
		debug.SetOutput(os.Stderr)

		if *syslogEnabled {
			mustEnableSyslog(debug, syslog.LOG_INFO, syslogTag)
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
	}

	svc = s3.New(nil)

	user := flag.Arg(0)

	go listenOnSigpipe()

	printAuthorizedKeys(*bucket, *key, user)
}

func mustEnableSyslog(logger *log.Logger, p syslog.Priority, tag string) {
	syslogWriter, err := syslog.New(p, tag)
	if err != nil {
		info.Fatalf("unable to initialize sysloger: %v", err)
	}
	logger.SetOutput(syslogWriter)
}

func listenOnSigpipe() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGPIPE)
	<-c
	debug.Printf("recived SIGPIPE signal: ignoring")
}

// readAuthorizedKey reads the authorized keys from S3
func readAuthorizedKey(bucket, key string, authorizedKeys chan io.Reader) {
	debug.Printf("Reading authorized key from bucket: %s, path: %s", bucket, key)
	params := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	resp, err := svc.GetObject(params)

	if err != nil {
		switch e := err.(type) {
		case awserr.Error:
			info.Printf("Unable to get authorized key from S3: %s: %s", e.Code(), e.Message())
		default:
			info.Printf("Unable to get authorized key from S3: %v", e)
		}
		authorizedKeys <- bytes.NewReader([]byte{})
		return
	}

	// Make sure that the the string ends with a new line
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		info.Printf("Unable to convert authorized key to byte array: %v", err)
	}
	if !bytes.HasSuffix(body, []byte("\n")) {
		body = append(body, []byte("\n")...)
	}

	if *authlog != "" {
		outbuf := bytes.NewBufferString(fmt.Sprintf(logheader, *authlog, path.Base(key), bucket, key))
		outbuf.Read(body)
		authorizedKeys <- outbuf
		return
	}

	authorizedKeys <- bytes.NewBuffer(body)
}

// printAuthorizedKeys for specified bucket, prefix (key) and user
// the used path will be bucket/prefix/user/*
func printAuthorizedKeys(bucket, authorizedKeysPath, user string) {
	authorizedKeys := make(chan io.Reader, 5)

	authorizedKeysPath = path.Join(authorizedKeysPath, user)
	debug.Printf("Listing authorized keys from bucket: %s, path: %s", bucket, authorizedKeysPath)

	params := &s3.ListObjectsInput{
		Bucket: aws.String(bucket),
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
				info.Printf("Unable to copy authorized key to stdout: %v", err)
			}
		}

		return !lastPage
	})

	if err != nil {
		switch e := err.(type) {
		case awserr.Error:
			info.Fatalf("Unable to list authorized keys: %s, message: %s", e.Code(), e.Message())
		default:
			info.Fatalf("Error listing authorized keys: %v", e)
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
	debug.Printf("Reading flag file: %s", flagFileName)

	flagFile, err := os.Open(flagFileName)
	if err != nil {
		dir, _ := os.Getwd()
		debug.Printf("Unable to open file: %s, directory: %s", flagFileName, dir)
		return
	}
	defer flagFile.Close()

	var args []string
	args = append(args, os.Args[0])

	// Read arguments from file
	scanner := bufio.NewScanner(flagFile)
	scanner.Split(bufio.ScanWords)
	for scanner.Scan() {
		args = append(args, scanner.Text())
	}

	// Add arguments from command line after
	for i := 1; i < len(os.Args); i++ {
		args = append(args, os.Args[i])
	}

	os.Args = args
}
