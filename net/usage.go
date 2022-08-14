package net

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	usage = "Toxnet server\nUsage:\n"
)

func Usage() {

	var outputFile string
	var payloadType string

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), usage)
		flag.PrintDefaults()
	}

	flag.StringVar(&outputFile, "o", "linux_payload", "Specify output file: -o [filename]")
	flag.StringVar(&payloadType, "t", "", "Generate a Toxnet payload: linux, windows")

	flag.Parse()

	if strings.ToLower(payloadType) == "linux" {
		GenerateLinuxStub(outputFile)
		os.Exit(0)
	}
}
