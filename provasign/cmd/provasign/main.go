package main

import (
	"os"

	"github.com/provasign/provasign/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
