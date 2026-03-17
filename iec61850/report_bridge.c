// report_bridge.c
// C-side bridge between libiec61850 report callbacks and the Go handler.
// Compiled as part of the iec61850 CGO package; inherits include paths from
// the platform config_*.go #cgo CFLAGS directives.

#include "iec61850_client.h"
#include "_cgo_export.h"

static void reportCallbackBridge(void* parameter, ClientReport report) {
    goReportCallback((GoInt)(intptr_t)parameter, (void*)report);
}

void installGoReportHandler(IedConnection conn, const char* rcbRef, const char* rptId,
                            int handlerID) {
    IedConnection_installReportHandler(conn, rcbRef, rptId, reportCallbackBridge,
                                       (void*)(intptr_t)handlerID);
}

void uninstallGoReportHandler(IedConnection conn, const char* rcbRef) {
    IedConnection_installReportHandler(conn, rcbRef, NULL, NULL, NULL);
}
