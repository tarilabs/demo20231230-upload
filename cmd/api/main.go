package main

import (
	"demo20231230-upload/internal/server"
	"log"
	"os"
)

func main() {
	log.Print("Starting...", os.Getenv("PORT"))
	if os.Getenv("PORT") == "" {
		os.Setenv("PORT", "8080")
	}
	server := server.NewServer()

	err := server.ListenAndServe()
	if err != nil {
		panic("cannot start server")
	}
}
