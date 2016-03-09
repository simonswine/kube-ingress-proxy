package main

import (
	log "github.com/Sirupsen/logrus"
)

const appName = "kube-inress-proxy"
const appVersion = "0.0.1"

func main() {
	ip := NewIngressProxy()

	err := ip.Init()
	if err != nil {
		log.Fatal("Error during initialization: ", err)
	}

	ip.Start()
}
