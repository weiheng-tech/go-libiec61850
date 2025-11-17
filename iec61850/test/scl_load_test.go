package test

import (
	"testing"

	"github.com/weiheng-tech/go-libiec61850/iec61850/scl_xml"
)

func TestIEC61850LoadICD(t *testing.T) {
	scl, err := scl_xml.GetSCL("test_icd.icd")
	if err != nil {
		t.Error(err)
	}
	scl.Print()
}
