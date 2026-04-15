package iec61850

// #include <iec61850_client.h>
import "C"
import (
	"fmt"
	"unsafe"
)

// cachedControl holds one ControlObjectClient and one reusable MmsValue for ctlVal (same type per ref).
type cachedControl struct {
	ctrl     C.ControlObjectClient
	ctlReuse *C.MmsValue
}

// clearControlObjectCache destroys cached ControlObjectClient handles and MmsValues before the connection is torn down.
func (client *IedClient) clearControlObjectCache() {
	client.controlMu.Lock()
	defer client.controlMu.Unlock()
	for _, e := range client.controlCache {
		if e == nil {
			continue
		}
		if e.ctlReuse != nil {
			C.MmsValue_delete(e.ctlReuse)
		}
		if e.ctrl != nil {
			C.ControlObjectClient_destroy(e.ctrl)
		}
	}
	client.controlCache = nil
}

// getOrCreateControlEntryLocked returns cached entry or creates ControlObjectClient. Caller must hold controlMu.
// If requireMmsInteger is true, ctlVal type must be MMS_INTEGER before caching; otherwise ctrl is destroyed.
func (client *IedClient) getOrCreateControlEntryLocked(controlReference string, requireMmsInteger bool) (*cachedControl, error) {
	if client.controlCache == nil {
		client.controlCache = make(map[string]*cachedControl)
	}
	if e := client.controlCache[controlReference]; e != nil {
		return e, nil
	}
	cRef := C.CString(controlReference)
	defer C.free(unsafe.Pointer(cRef))
	ctrl := C.ControlObjectClient_create(cRef, client.connection)
	if ctrl == nil {
		return nil, fmt.Errorf("error creating control object client for %q", controlReference)
	}
	if requireMmsInteger {
		got := MMSType(C.ControlObjectClient_getCtlValType(ctrl))
		if got != MMS_INTEGER {
			C.ControlObjectClient_destroy(ctrl)
			return nil, fmt.Errorf("control %q: ctlVal MMS type is %d (%s), want MMS_INTEGER for INC",
				controlReference, got, mmsTypeName(got))
		}
	}
	e := &cachedControl{ctrl: ctrl}
	client.controlCache[controlReference] = e
	return e, nil
}

// DirectWithNormalSecurity issues a direct-with-normal-security Operate for an SPC-style
// control object (boolean ctlVal). controlReference is the DO path without FC suffix
// (e.g. "LD/LN.start", not "...start.stVal").
func (client *IedClient) DirectWithNormalSecurity(controlReference string, val bool) error {
	client.controlMu.Lock()
	defer client.controlMu.Unlock()

	e, err := client.getOrCreateControlEntryLocked(controlReference, false)
	if err != nil {
		return err
	}
	if e.ctlReuse == nil {
		e.ctlReuse = C.MmsValue_newBoolean(C._Bool(false))
		if e.ctlReuse == nil {
			return fmt.Errorf("control %q: MmsValue_newBoolean failed", controlReference)
		}
	}
	C.MmsValue_setBoolean(e.ctlReuse, C._Bool(val))

	C.ControlObjectClient_setOrigin(e.ctrl, nil, 3)

	if bool(C.ControlObjectClient_operate(e.ctrl, e.ctlReuse, 0)) {
		return nil
	}
	errCode := C.ControlObjectClient_getLastError(e.ctrl)
	return fmt.Errorf("failed to operate %q: %s", controlReference, Err(errCode))
}

// DirectWithNormalSecurityInt32 issues a direct-with-normal-security Operate for an INC-style
// control object (integer ctlVal, MMS_INTEGER). controlReference is the DO path without FC suffix.
// If the server's ctlVal is not MMS_INTEGER (e.g. still boolean SPC), this returns an error without sending Operate.
func (client *IedClient) DirectWithNormalSecurityInt32(controlReference string, val int32) error {
	client.controlMu.Lock()
	defer client.controlMu.Unlock()

	e, err := client.getOrCreateControlEntryLocked(controlReference, true)
	if err != nil {
		return err
	}
	if e.ctlReuse == nil {
		e.ctlReuse = C.MmsValue_newIntegerFromInt32(0)
		if e.ctlReuse == nil {
			return fmt.Errorf("control %q: MmsValue_newIntegerFromInt32 failed", controlReference)
		}
	}
	C.MmsValue_setInt32(e.ctlReuse, C.int32_t(val))

	C.ControlObjectClient_setOrigin(e.ctrl, nil, 3)

	if bool(C.ControlObjectClient_operate(e.ctrl, e.ctlReuse, 0)) {
		return nil
	}
	errCode := C.ControlObjectClient_getLastError(e.ctrl)
	return fmt.Errorf("failed to operate %q: %s", controlReference, Err(errCode))
}

// DirectWithNormalSecurityFloat32 issues a direct-with-normal-security Operate for APC float setpoints.
// Uses MmsValue_newFloat (same as cmd/apcctlclient). libIEC61850 maps this to the server Oper type;
// ControlObjectClient_getCtlValType may report MMS_STRUCTURE even though a plain float is accepted.
func (client *IedClient) DirectWithNormalSecurityFloat32(controlReference string, val float32) error {
	client.controlMu.Lock()
	defer client.controlMu.Unlock()

	e, err := client.getOrCreateControlEntryLocked(controlReference, false)
	if err != nil {
		return err
	}
	if e.ctlReuse == nil {
		e.ctlReuse = C.MmsValue_newFloat(0)
		if e.ctlReuse == nil {
			return fmt.Errorf("control %q: MmsValue_newFloat failed", controlReference)
		}
	}
	C.MmsValue_setFloat(e.ctlReuse, C.float(val))

	C.ControlObjectClient_setOrigin(e.ctrl, nil, 3)

	if bool(C.ControlObjectClient_operate(e.ctrl, e.ctlReuse, 0)) {
		return nil
	}
	errCode := C.ControlObjectClient_getLastError(e.ctrl)
	return fmt.Errorf("failed to operate %q: %s", controlReference, Err(errCode))
}

func mmsTypeName(t MMSType) string {
	switch t {
	case MMS_ARRAY:
		return "MMS_ARRAY"
	case MMS_STRUCTURE:
		return "MMS_STRUCTURE"
	case MMS_BOOLEAN:
		return "MMS_BOOLEAN"
	case MMS_BIT_STRING:
		return "MMS_BIT_STRING"
	case MMS_INTEGER:
		return "MMS_INTEGER"
	case MMS_UNSIGNED:
		return "MMS_UNSIGNED"
	case MMS_FLOAT:
		return "MMS_FLOAT"
	case MMS_OCTET_STRING:
		return "MMS_OCTET_STRING"
	case MMS_VISIBLE_STRING:
		return "MMS_VISIBLE_STRING"
	case MMS_GENERALIZED_TIME:
		return "MMS_GENERALIZED_TIME"
	case MMS_BINARY_TIME:
		return "MMS_BINARY_TIME"
	case MMS_BCD:
		return "MMS_BCD"
	case MMS_OBJ_ID:
		return "MMS_OBJ_ID"
	case MMS_STRING:
		return "MMS_STRING"
	case MMS_UTC_TIME:
		return "MMS_UTC_TIME"
	case MMS_DATA_ACCESS_ERROR:
		return "MMS_DATA_ACCESS_ERROR"
	default:
		return "unknown"
	}
}
