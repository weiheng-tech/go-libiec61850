package test

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/weiheng-tech/go-libiec61850/iec61850"
)

// TestDirectWithNormalSecurityInt32 requires a running MMS server with an INC control object.
// Example: rtserver with EMS CID and ref like EMSCTRL/GGIO1.direct_required_active_power
//
//	INC_CONTROL_TEST=1 INC_HOST=127.0.0.1 INC_PORT=18102 \
//	  INC_REF=EMSCTRL/GGIO1.direct_required_active_power \
//	  go test ./iec61850/test -run TestDirectWithNormalSecurityInt32 -v
//
// Without INC_CONTROL_TEST=1, the test is skipped.
func TestDirectWithNormalSecurityInt32(t *testing.T) {
	if os.Getenv("INC_CONTROL_TEST") != "1" {
		t.Skip("set INC_CONTROL_TEST=1 and point at a running IED with INC (see client_control_int32_test.go header)")
	}

	host := getenvDefault("INC_HOST", "127.0.0.1")
	portStr := getenvDefault("INC_PORT", "18102")
	ref := getenvDefault("INC_REF", "EMSCTRL/GGIO1.direct_required_active_power")

	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum <= 0 || portNum > 65535 {
		t.Fatalf("INC_PORT invalid: %q", portStr)
	}

	client := iec61850.NewIedClient(iec61850.ConnectTimeout(5*time.Second), iec61850.RequestTimeout(30*time.Second))
	if err := client.Connect(host, portNum); err != nil {
		t.Fatalf("connect %s:%d: %v", host, portNum, err)
	}
	defer client.Close()

	stPath := ref + ".stVal"
	before, err := client.ReadInt32(stPath, iec61850.IEC61850_FC_ST)
	if err != nil {
		t.Fatalf("read before %s: %v", stPath, err)
	}
	t.Logf("before: %s = %d", stPath, before)

	const want int32 = 42
	if err := client.DirectWithNormalSecurityInt32(ref, want); err != nil {
		t.Fatalf("DirectWithNormalSecurityInt32: %v", err)
	}

	after, err := client.ReadInt32(stPath, iec61850.IEC61850_FC_ST)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	t.Logf("after: %s = %d", stPath, after)
	if after != want {
		t.Fatalf("stVal got %d want %d (server may map scale or reject)", after, want)
	}
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
