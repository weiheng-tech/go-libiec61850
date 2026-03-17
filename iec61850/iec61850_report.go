package iec61850

// The preamble of a file with //export must contain only declarations.
// Function definitions for installGoReportHandler/uninstallGoReportHandler
// live in report_bridge.c which is compiled alongside this package.

/*
#include <iec61850_client.h>
extern void installGoReportHandler(IedConnection conn, const char* rcbRef,
                                   const char* rptId, int handlerID);
extern void uninstallGoReportHandler(IedConnection conn, const char* rcbRef);
*/
import "C"
import (
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Trigger options for a Report Control Block (matches iec61850_common.h TRG_OPT_*).
const (
	TRG_OPT_DATA_CHANGED    = 1
	TRG_OPT_QUALITY_CHANGED = 2
	TRG_OPT_DATA_UPDATE     = 4
	TRG_OPT_INTEGRITY       = 8
	TRG_OPT_GI              = 16
)

// Report optional fields (matches iec61850_common.h RPT_OPT_*).
const (
	RPT_OPT_SEQ_NUM            = 1
	RPT_OPT_TIME_STAMP         = 2
	RPT_OPT_REASON_FOR_INCLUDE = 4
	RPT_OPT_DATA_SET           = 8
	RPT_OPT_DATA_REFERENCE     = 16
	RPT_OPT_BUFFER_OVERFLOW    = 32
	RPT_OPT_ENTRY_ID           = 64
	RPT_OPT_CONF_REV           = 128
)

// ReasonForInclusion describes why a data element was included in a report.
// Values match IEC61850_REASON_* defines in iec61850_client.h.
type ReasonForInclusion int

const (
	REASON_NOT_INCLUDED ReasonForInclusion = 0
	REASON_DATA_CHANGE  ReasonForInclusion = 1
	REASON_QUAL_CHANGE  ReasonForInclusion = 2
	REASON_DATA_UPDATE  ReasonForInclusion = 4
	REASON_INTEGRITY    ReasonForInclusion = 8
	REASON_GI           ReasonForInclusion = 16
	REASON_UNKNOWN      ReasonForInclusion = 32
)

// ReportEntry holds one element from a received report.
type ReportEntry struct {
	Value  interface{}
	Reason ReasonForInclusion
}

// Report holds the contents of a received IEC 61850 report.
type Report struct {
	RcbRef     string
	DataSetRef string
	SequenceNo uint16
	// Timestamp is milliseconds since Unix epoch (0 if not included in report).
	Timestamp uint64
	Entries   []ReportEntry
}

// ReportHandler is invoked on each received report.
// WARNING: do not call IedClient methods that send MMS requests from within
// this callback — the underlying connection is locked and a deadlock will occur.
// Dispatch to a goroutine if further reads are needed.
type ReportHandler func(report *Report)

// ReportConfig holds optional overrides for a Report Control Block.
// Zero values leave the server's current RCB settings unchanged.
type ReportConfig struct {
	// TrgOps overrides trigger options. Use TRG_OPT_* constants combined with |.
	TrgOps int
	// IntgPd overrides the integrity period in milliseconds.
	IntgPd uint32
	// BufTm overrides the buffer time in milliseconds.
	BufTm uint32
	// GI triggers a General Interrogation immediately after enabling reporting.
	GI bool
}

// --- internal registry ---

var (
	reportMu      sync.RWMutex
	reportHandlers = make(map[int]reportHandlerEntry)
	nextHandlerID  atomic.Int32
)

type reportHandlerEntry struct {
	handler ReportHandler
	rcbRef  string
}

// goReportCallback is the Go-side entry point called from report_bridge.c.
//
//export goReportCallback
func goReportCallback(handlerID C.int, reportPtr unsafe.Pointer) {
	reportMu.RLock()
	entry, ok := reportHandlers[int(handlerID)]
	reportMu.RUnlock()
	if !ok {
		return
	}

	cReport := C.ClientReport(reportPtr)
	report := buildReport(entry.rcbRef, cReport)
	entry.handler(report)
}

func buildReport(rcbRef string, cr C.ClientReport) *Report {
	r := &Report{RcbRef: rcbRef}

	if ds := C.ClientReport_getDataSetName(cr); ds != nil {
		r.DataSetRef = C.GoString(ds)
	}
	if bool(C.ClientReport_hasSeqNum(cr)) {
		r.SequenceNo = uint16(C.ClientReport_getSeqNum(cr))
	}
	if bool(C.ClientReport_hasTimestamp(cr)) {
		r.Timestamp = uint64(C.ClientReport_getTimestamp(cr))
	}

	values := C.ClientReport_getDataSetValues(cr)
	if values == nil {
		return r
	}

	hasReason := bool(C.ClientReport_hasReasonForInclusion(cr))

	// Iterate dataset elements until MmsValue_getElement returns nil.
	for i := 0; ; i++ {
		val := C.MmsValue_getElement(values, C.int(i))
		if val == nil {
			break
		}
		entry := ReportEntry{}
		if hasReason {
			entry.Reason = ReasonForInclusion(C.ClientReport_getReasonForInclusion(cr, C.int(i)))
		}
		entry.Value = extractReportValue(val)
		r.Entries = append(r.Entries, entry)
	}

	return r
}

// extractReportValue converts any MmsValue to a Go value.
// Structures are returned as []interface{} so the caller can correlate
// entries with a DataSetSchema if needed.
func extractReportValue(val *C.MmsValue) interface{} {
	switch MMSType(C.MmsValue_getType(val)) {
	case MMS_BOOLEAN:
		return bool(C.MmsValue_getBoolean(val))
	case MMS_FLOAT:
		return float64(C.MmsValue_toDouble(val))
	case MMS_INTEGER, MMS_UNSIGNED:
		return int64(C.MmsValue_toInt64(val))
	case MMS_STRING, MMS_VISIBLE_STRING:
		if s := C.MmsValue_toString(val); s != nil {
			return C.GoString(s)
		}
		return ""
	case MMS_BIT_STRING:
		return uint32(C.MmsValue_getBitStringAsInteger(val))
	case MMS_UTC_TIME:
		return uint32(C.MmsValue_toUnixTimestamp(val))
	case MMS_STRUCTURE, MMS_ARRAY:
		var subs []interface{}
		for i := 0; ; i++ {
			sub := C.MmsValue_getElement(val, C.int(i))
			if sub == nil {
				break
			}
			subs = append(subs, extractReportValue(sub))
		}
		return subs
	}
	return nil
}

// SubscribeReport configures and enables a Report Control Block on the server,
// then installs handler to receive reports.
//
// rcbRef format:
//
//	unbuffered: "IEDNameLDInst/LNName.RP.RCBName"   (e.g. "testSENSORS/LLN0.RP.events01")
//	buffered:   "IEDNameLDInst/LNName.BR.RCBName"
//
// cfg may be nil to keep all server-side RCB defaults.
func (client *IedClient) SubscribeReport(rcbRef string, cfg *ReportConfig, handler ReportHandler) error {
	cRcbRef := C.CString(rcbRef)
	defer C.free(unsafe.Pointer(cRcbRef))

	var clientError C.IedClientError
	rcb := C.IedConnection_getRCBValues(client.connection, &clientError, cRcbRef, nil)
	if clientError != C.IED_ERROR_OK {
		return fmt.Errorf("getRCBValues %s: %s", rcbRef, Err(clientError))
	}
	defer C.ClientReportControlBlock_destroy(rcb)

	// Build the write mask — start with RptEna.
	mask := C.uint32_t(C.RCB_ELEMENT_RPT_ENA)

	if cfg != nil {
		if cfg.TrgOps != 0 {
			C.ClientReportControlBlock_setTrgOps(rcb, C.int(cfg.TrgOps))
			mask |= C.uint32_t(C.RCB_ELEMENT_TRG_OPS)
		}
		if cfg.IntgPd != 0 {
			C.ClientReportControlBlock_setIntgPd(rcb, C.uint32_t(cfg.IntgPd))
			mask |= C.uint32_t(C.RCB_ELEMENT_INTG_PD)
		}
		if cfg.BufTm != 0 {
			C.ClientReportControlBlock_setBufTm(rcb, C.uint32_t(cfg.BufTm))
			mask |= C.uint32_t(C.RCB_ELEMENT_BUF_TM)
		}
		if cfg.GI {
			C.ClientReportControlBlock_setGI(rcb, C.bool(true))
			mask |= C.uint32_t(C.RCB_ELEMENT_GI)
		}
	}

	C.ClientReportControlBlock_setRptEna(rcb, C.bool(true))

	// Register the Go handler before installing the C callback to avoid a window
	// where reports arrive but no handler is registered.
	id := int(nextHandlerID.Add(1))
	reportMu.Lock()
	reportHandlers[id] = reportHandlerEntry{handler: handler, rcbRef: rcbRef}
	reportMu.Unlock()

	rptId := C.ClientReportControlBlock_getRptId(rcb)
	C.installGoReportHandler(client.connection, cRcbRef, rptId, C.int(id))

	C.IedConnection_setRCBValues(client.connection, &clientError, rcb, mask, C.bool(true))
	if clientError != C.IED_ERROR_OK {
		// Roll back: remove handler and uninstall C callback.
		reportMu.Lock()
		delete(reportHandlers, id)
		reportMu.Unlock()
		C.uninstallGoReportHandler(client.connection, cRcbRef)
		return fmt.Errorf("setRCBValues %s: %s", rcbRef, Err(clientError))
	}

	return nil
}

// UnsubscribeReport disables reporting for an RCB and removes the local handler.
// It is safe to call even if SubscribeReport was never called for rcbRef.
func (client *IedClient) UnsubscribeReport(rcbRef string) {
	cRcbRef := C.CString(rcbRef)
	defer C.free(unsafe.Pointer(cRcbRef))

	// Ask the server to stop sending reports.
	var clientError C.IedClientError
	rcb := C.IedConnection_getRCBValues(client.connection, &clientError, cRcbRef, nil)
	if clientError == C.IED_ERROR_OK {
		C.ClientReportControlBlock_setRptEna(rcb, C.bool(false))
		C.IedConnection_setRCBValues(client.connection, &clientError, rcb,
			C.uint32_t(C.RCB_ELEMENT_RPT_ENA), C.bool(true))
		C.ClientReportControlBlock_destroy(rcb)
	}

	C.uninstallGoReportHandler(client.connection, cRcbRef)

	reportMu.Lock()
	for id, entry := range reportHandlers {
		if entry.rcbRef == rcbRef {
			delete(reportHandlers, id)
			break
		}
	}
	reportMu.Unlock()
}
