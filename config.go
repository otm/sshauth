package main

import (
	"bufio"
	"flag"
	"os"
)

var (
	bucket = flag.String("bucket", "", "S3 Bucket name")
	key    = flag.String("key", "", "S3 bucket key")
)

func readDefaultFlagFile() {
	readFlagFile(flagFileName)
}

func readFlagFile(flagFileName string) {
	flagFile, err := os.Open(flagFileName)
	if err != nil {
		dir, _ := os.Getwd()
		printDbg("Unable to open file: ", flagFileName, ", In folder:", dir)
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
