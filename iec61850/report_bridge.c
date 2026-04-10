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

// Flat structure representation for minimizing cgo crossover
#include <stdlib.h>
#include <stdbool.h>

// Type markers to represent structures/arrays
#define FLAT_STRUCT_START 100
#define FLAT_STRUCT_END 101
#define FLAT_ARRAY_START 102
#define FLAT_ARRAY_END 103



static int count_mms_elements(MmsValue* val) {
    if (!val) return 0;
    int t = MmsValue_getType(val);
    if (t == MMS_STRUCTURE || t == MMS_ARRAY) {
        int count = 2; // start, end
        int size = MmsValue_getArraySize(val);
        for(int i = 0; i < size; i++) {
            count += count_mms_elements(MmsValue_getElement(val, i));
        }
        return count;
    }
    return 1;
}

static void flatten_mms_value(MmsValue* val, FlatMmsValue* arr, int* index, int max_len) {
    if (*index >= max_len) return;
    if (!val) return;
    
    int t = MmsValue_getType(val);
    arr[*index].typ = t;
    
    if (t == MMS_BOOLEAN) {
        arr[*index].ival = MmsValue_getBoolean(val);
        (*index)++;
    } else if (t == MMS_FLOAT) {
        arr[*index].dval = (double)MmsValue_toFloat(val);
        (*index)++;
    } else if (t == MMS_INTEGER || t == MMS_UNSIGNED) {
        arr[*index].ival = MmsValue_toInt64(val);
        (*index)++;
    } else if (t == MMS_STRING || t == MMS_VISIBLE_STRING || t == MMS_OCTET_STRING) {
        arr[*index].sval = MmsValue_toString(val);
        (*index)++;
    } else if (t == MMS_BIT_STRING) {
        arr[*index].ival = MmsValue_getBitStringAsInteger(val);
        (*index)++;
    } else if (t == MMS_UTC_TIME) {
        arr[*index].ival = MmsValue_toUint32(val); // MZ library API is often something like MmsValue_toUnixTimestamp 
        // Wait, MmsValue_toUnixTimestamp was used in the Go file `C.MmsValue_toUnixTimestamp(val)`
        // I will match it closely.
        arr[*index].ival = MmsValue_toUnixTimestamp(val);
        (*index)++;
    } else if (t == MMS_STRUCTURE || t == MMS_ARRAY) {
        (*index)++;
        
        int size = MmsValue_getArraySize(val);
        for(int i = 0; i < size; i++) {
            MmsValue* sub = MmsValue_getElement(val, i);
            flatten_mms_value(sub, arr, index, max_len);
        }
        
        if (*index < max_len) {
            (*index)++;
        }
    } else {
        (*index)++;
    }
}

FlatMmsValue* flattenReportDataSet(ClientReport report, int* out_count) {
    MmsValue* values = ClientReport_getDataSetValues(report);
    if (!values) {
        *out_count = 0;
        return NULL;
    }
    
    int count = 0;
    int i = 0;
    while (true) {
        MmsValue* val = MmsValue_getElement(values, i);
        if (!val) break;
        count += count_mms_elements(val);
        i++;
    }
    
    if (count == 0) {
        *out_count = 0;
        return NULL;
    }
    
    FlatMmsValue* arr = (FlatMmsValue*)malloc(sizeof(FlatMmsValue) * count);
    if (!arr) {
        *out_count = 0;
        return NULL;
    }
    
    int index = 0;
    i = 0;
    while (true) {
        MmsValue* val = MmsValue_getElement(values, i);
        if (!val) break;
        flatten_mms_value(val, arr, &index, count);
        i++;
    }
    
    *out_count = index;
    return arr;
}
