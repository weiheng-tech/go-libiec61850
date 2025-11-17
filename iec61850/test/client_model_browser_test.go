package test

import (
	"fmt"
	"testing"

	"github.com/weiheng-tech/go-libiec61850/iec61850"
)

func TestIEC61850ClientPrintTree(t *testing.T) {
	client := iec61850.NewIedClient()
	err := client.Connect("localhost", 102)
	if err != nil {
		fmt.Println("Error connecting:", err)
		return
	}
	fmt.Println("BROWSER MODEL....................")
	client.BrowseModel()
	scl, err := client.BrowseModelToSCL()
	if err != nil {
		t.Error(err)
	}
	fmt.Println("BROWSER SCL MODEL................")
	scl.Print()

	client.Close()
}
