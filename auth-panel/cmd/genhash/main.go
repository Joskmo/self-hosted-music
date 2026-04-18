package main

import (
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run ./cmd/genhash <password>")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(os.Args[1]), 12)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(h))
}
