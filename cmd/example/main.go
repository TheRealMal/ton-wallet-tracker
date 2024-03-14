package main

import (
	"ton-wallet-tracker/pkg/observer"
)

func main() {
	obs := observer.InitObserver()
	obs.Observe()
}
