package main

import (
	"os"

	"github.com/agentconnect/awiki-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
