package main

import (
	"os"

	"github.com/AGubenskiy/GoferMart/internal/entrypoint"
)

func main() {
	os.Exit(entrypoint.RunGophermart(os.Args[1:], os.Stderr))
}
