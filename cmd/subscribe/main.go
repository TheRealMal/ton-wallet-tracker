package main

import (
	"os"
	"ton-wallet-tracker/pkg/observer"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load(".env")
	obs := observer.InitObserver(os.Getenv("TELEGRAM_TOKEN"), []int64{558161625, 162332155})
	obs.Observe("EQCXwWAyDG_IhRh6CzPSetvgGecywZBU3YNCawmz03Uk25RG")
}
