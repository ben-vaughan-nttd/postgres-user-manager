package main

import (
	"fmt"
	"os"

	"github.com/ben-vaughan-nttd/postgres-user-manager/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
