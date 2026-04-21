package iec61850

// The preamble of a file with //export must contain only declarations.
// Function definitions for installGoReportHandler/uninstallGoReportHandler
// live in report_bridge.c which is compiled alongside this package.

/*
#include <stdlib.h>
#include <iec61850_client.h>
extern void installGoReportHandler(IedConnection conn, const char* rcbRef,
                                   const char* rptId, int handlerID);
extern void uninstallGoReportHandler(IedConnection conn, const char* rcbRef);

typedef struct {
    int typ;
    int64_t ival;
    double dval;
    const char* sval;
} FlatMmsValue;

extern FlatMmsValue* flattenReportDataSet(ClientReport report, int* out_count);
*/
import "C"
import (
	"fmt"
	"strings"
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

// String returns a human-readable bitmask form of the reason,
// e.g. "DATA_CHANGE|INTEGRITY". Zero returns "0"; unknown bits fall back to "raw=N".
func (r ReasonForInclusion) String() string {
	ri := int(r)
	if ri == 0 {
		return "0"
	}
	var parts []string
	if ri&int(REASON_DATA_CHANGE) != 0 {
		parts = append(parts, "DATA_CHANGE")
	}
	if ri&int(REASON_QUAL_CHANGE) != 0 {
		parts = append(parts, "QUAL_CHANGE")
	}
	if ri&int(REASON_DATA_UPDATE) != 0 {
		parts = append(parts, "DATA_UPDATE")
	}
	if ri&int(REASON_INTEGRITY) != 0 {
		parts = append(parts, "INTEGRITY")
	}
	if ri&int(REASON_GI) != 0 {
		parts = append(parts, "GI")
	}
	if ri&int(REASON_UNKNOWN) != 0 {
		parts = append(parts, "UNKNOWN")
	}
	if len(parts) == 0 {
		return fmt.Sprintf("raw=%d", ri)
	}
	return strings.Join(parts, "|")
}

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

	var count C.int
	cArr := C.flattenReportDataSet(cr, &count)
	if cArr == nil || count <= 0 {
		return r
	}
	defer C.free(unsafe.Pointer(cArr))

	flatSlice := unsafe.Slice((*C.FlatMmsValue)(unsafe.Pointer(cArr)), int(count))
	pos := 0

	hasReason := bool(C.ClientReport_hasReasonForInclusion(cr))

	for i := 0; pos < len(flatSlice); i++ {
		entry := ReportEntry{}
		if hasReason {
			entry.Reason = ReasonForInclusion(C.ClientReport_getReasonForInclusion(cr, C.int(i)))
		}
		entry.Value = extractFlatValue(flatSlice, &pos)
		r.Entries = append(r.Entries, entry)
	}

	return r
}

// extractFlatValue converts a flat slice of C.FlatMmsValue tokens back to a recursive Go value.
// Structures are returned as []interface{} so the caller can correlate
// entries with a DataSetSchema if needed.
func extractFlatValue(flatSlice []C.FlatMmsValue, pos *int) interface{} {
	if *pos >= len(flatSlice) {
		return nil
	}
	elem := flatSlice[*pos]
	*pos++

	switch int(elem.typ) {
	case int(MMS_BOOLEAN):
		return elem.ival != 0
	case int(MMS_FLOAT):
		return float64(elem.dval)
	case int(MMS_INTEGER), int(MMS_UNSIGNED):
		return int64(elem.ival)
	case int(MMS_STRING), int(MMS_VISIBLE_STRING), int(MMS_OCTET_STRING): // Add OCTET string match if needed
		if elem.sval != nil {
			return C.GoString(elem.sval)
		}
		return ""
	case int(MMS_BIT_STRING):
		return uint32(elem.ival)
	case int(MMS_UTC_TIME):
		return uint32(elem.ival)
	case 100, 102: // FLAT_STRUCT_START, FLAT_ARRAY_START
		var subs []interface{}
		for *pos < len(flatSlice) {
			typ := int(flatSlice[*pos].typ)
			if typ == 101 || typ == 103 { // FLAT_STRUCT_END, FLAT_ARRAY_END
				*pos++
				break
			}
			subs = append(subs, extractFlatValue(flatSlice, pos))
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

// SubscribeReportWithCandidates tries a list of RCB reference candidates in order,
// returning the first one that subscribes successfully.
//
// For each candidate, if the initial SubscribeReport fails, the method also tries
// the same ref with a trailing "01" suffix (common indexed-instance convention).
//
// Typical callers use this when the server's exact RCB reference is not known
// ahead of time (e.g. BR vs RP naming, vendor-specific LD/LN prefixes discovered
// from CID). All candidate refs share the same cfg and handler.
//
// Returns the chosenRef that succeeded; on full failure, returns an error that
// concatenates per-candidate error messages (up to the first 20 for readability).
func (client *IedClient) SubscribeReportWithCandidates(candidates []string, cfg *ReportConfig, handler ReportHandler) (chosenRef string, err error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("SubscribeReportWithCandidates: no candidates")
	}
	const errMsgMax = 20
	var errs []string
	for _, base := range candidates {
		if e := client.SubscribeReport(base, cfg, handler); e == nil {
			return base, nil
		} else if len(errs) < errMsgMax {
			errs = append(errs, fmt.Sprintf("%s: %v", base, e))
		}
		alt := base + "01"
		if e := client.SubscribeReport(alt, cfg, handler); e == nil {
			return alt, nil
		} else if len(errs) < errMsgMax {
			errs = append(errs, fmt.Sprintf("%s: %v", alt, e))
		}
	}
	return "", fmt.Errorf("SubscribeReportWithCandidates: all %d refs (with +01 alt) failed: %s",
		len(candidates), strings.Join(errs, "; "))
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
