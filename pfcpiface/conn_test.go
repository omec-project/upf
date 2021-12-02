package main

import (
	log "github.com/sirupsen/logrus"
	"github.com/wmnsk/go-pfcp/ie"
	"testing"
)

func Test(t *testing.T) {
	buf := []byte{0, 0, 0, 16, 0}

	parsedIe, err := ie.Parse(buf)
	if err != nil {
		log.Error(err)
		t.FailNow()
	}
	log.Warnln("Type:", parsedIe.Type)
	log.Warnln("Length:", parsedIe.Length)
	log.Warnln(parsedIe)
}
