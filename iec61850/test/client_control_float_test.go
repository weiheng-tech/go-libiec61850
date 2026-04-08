package test

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/weiheng-tech/go-libiec61850/iec61850"
)

// TestDirectWithNormalSecurityFloat32 requires a running MMS server whose control ctlVal is MMS_FLOAT
// (e.g. go-iec61850 cmd/ctlfloatdemo: DEMOCTRL/GGIO1.demo_sp).
//
//	APC_FLOAT_TEST=1 APC_FLOAT_HOST=127.0.0.1 APC_FLOAT_PORT=102 APC_FLOAT_REF=DEMOCTRL/GGIO1.demo_sp \
//	  go test ./iec61850/test -run TestDirectWithNormalSecurityFloat32 -v
func TestDirectWithNormalSecurityFloat32(t *testing.T) {
	if os.Getenv("APC_FLOAT_TEST") != "1" {
		t.Skip("set APC_FLOAT_TEST=1 and run ctlfloatdemo (see client_control_float_test.go header)")
	}

	host := getenvDefault("APC_FLOAT_HOST", "127.0.0.1")
	portStr := getenvDefault("APC_FLOAT_PORT", "102")
	ref := getenvDefault("APC_FLOAT_REF", "DEMOCTRL/GGIO1.demo_sp")

	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum <= 0 || portNum > 65535 {
		t.Fatalf("APC_FLOAT_PORT invalid: %q", portStr)
	}

	client := iec61850.NewIedClient(iec61850.ConnectTimeout(5*time.Second), iec61850.RequestTimeout(30*time.Second))
	if err := client.Connect(host, portNum); err != nil {
		t.Fatalf("connect %s:%d: %v", host, portNum, err)
	}
	defer client.Close()

	const want float32 = 2.718
	if err := client.DirectWithNormalSecurityFloat32(ref, want); err != nil {
		t.Fatalf("DirectWithNormalSecurityFloat32: %v", err)
	}

	magPath := ref + ".mag.f"
	v, err := client.ReadFloat(magPath, iec61850.IEC61850_FC_MX)
	if err != nil {
		t.Logf("read %s (MX) after (optional): %v", magPath, err)
		return
	}
	if float32(v) != want {
		t.Fatalf("mag.f got %g want %g", v, want)
	}
}
