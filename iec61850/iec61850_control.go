package iec61850

// #include <iec61850_client.h>
import "C"
import (
	"fmt"
	"unsafe"
)

// DirectWithNormalSecurity issues a direct-with-normal-security Operate for an SPC-style
// control object (boolean ctlVal). controlReference is the DO path without FC suffix
// (e.g. "LD/LN.start", not "...start.stVal").
func (client *IedClient) DirectWithNormalSecurity(controlReference string, val bool) error {
	cDataSetReference := C.CString(controlReference)
	defer C.free(unsafe.Pointer(cDataSetReference))

	control := C.ControlObjectClient_create(cDataSetReference, client.connection)
	if control == nil {
		return fmt.Errorf("error creating control object client for %q", controlReference)
	}

	defer C.ControlObjectClient_destroy(control)

	ctlVal := C.MmsValue_newBoolean(C._Bool(val))
	defer C.MmsValue_delete(ctlVal)

	C.ControlObjectClient_setOrigin(control, nil, 3)

	if bool(C.ControlObjectClient_operate(control, ctlVal, 0)) {
		return nil
	}
	errCode := C.ControlObjectClient_getLastError(control)
	return fmt.Errorf("failed to operate %q: %s", controlReference, Err(errCode))
}

// DirectWithNormalSecurityInt32 issues a direct-with-normal-security Operate for an INC-style
// control object (integer ctlVal, MMS_INTEGER). controlReference is the DO path without FC suffix.
// If the server's ctlVal is not MMS_INTEGER (e.g. still boolean SPC), this returns an error without sending Operate.
func (client *IedClient) DirectWithNormalSecurityInt32(controlReference string, val int32) error {
	cRef := C.CString(controlReference)
	defer C.free(unsafe.Pointer(cRef))

	control := C.ControlObjectClient_create(cRef, client.connection)
	if control == nil {
		return fmt.Errorf("error creating control object client for %q", controlReference)
	}
	defer C.ControlObjectClient_destroy(control)

	got := MMSType(C.ControlObjectClient_getCtlValType(control))
	if got != MMS_INTEGER {
		return fmt.Errorf("control %q: ctlVal MMS type is %d (%s), want MMS_INTEGER for INC",
			controlReference, got, mmsTypeName(got))
	}

	ctlVal := C.MmsValue_newIntegerFromInt32(C.int32_t(val))
	if ctlVal == nil {
		return fmt.Errorf("control %q: MmsValue_newIntegerFromInt32 failed", controlReference)
	}
	defer C.MmsValue_delete(ctlVal)

	C.ControlObjectClient_setOrigin(control, nil, 3)

	if bool(C.ControlObjectClient_operate(control, ctlVal, 0)) {
		return nil
	}
	errCode := C.ControlObjectClient_getLastError(control)
	return fmt.Errorf("failed to operate %q: %s", controlReference, Err(errCode))
}

// DirectWithNormalSecurityFloat32 issues a direct-with-normal-security Operate for APC float setpoints.
// Uses MmsValue_newFloat (same as cmd/apcctlclient). libIEC61850 maps this to the server Oper type;
// ControlObjectClient_getCtlValType may report MMS_STRUCTURE even though a plain float is accepted.
func (client *IedClient) DirectWithNormalSecurityFloat32(controlReference string, val float32) error {
	cRef := C.CString(controlReference)
	defer C.free(unsafe.Pointer(cRef))

	control := C.ControlObjectClient_create(cRef, client.connection)
	if control == nil {
		return fmt.Errorf("error creating control object client for %q", controlReference)
	}
	defer C.ControlObjectClient_destroy(control)

	ctlVal := C.MmsValue_newFloat(C.float(val))
	if ctlVal == nil {
		return fmt.Errorf("control %q: MmsValue_newFloat failed", controlReference)
	}
	defer C.MmsValue_delete(ctlVal)

	C.ControlObjectClient_setOrigin(control, nil, 3)

	if bool(C.ControlObjectClient_operate(control, ctlVal, 0)) {
		return nil
	}
	errCode := C.ControlObjectClient_getLastError(control)
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
