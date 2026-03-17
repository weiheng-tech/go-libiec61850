package iec61850

/*
#include "iec61850_server.h"
*/
import "C"
import "unsafe"

type IedModel struct {
	model *C.IedModel
}

func NewIedModel(name string) *IedModel {
	return &IedModel{
		model: C.IedModel_create(C.CString(name)),
	}
}

func (m *IedModel) Destroy() {
	C.IedModel_destroy(m.model)
}

type LogicalDevice struct {
	device *C.LogicalDevice
}

func (m *IedModel) CreateLogicalDevice(name string) *LogicalDevice {
	return &LogicalDevice{
		device: C.LogicalDevice_create(C.CString(name), m.model),
	}
}

type LogicalNode struct {
	node *C.LogicalNode
}

func (d *LogicalDevice) CreateLogicalNode(name string) *LogicalNode {
	return &LogicalNode{
		node: C.LogicalNode_create(C.CString(name), d.device),
	}
}

type DataObject struct {
	object *C.DataObject
}

// ENS: EnumerationString
// VSS: Visible String Setting
// SAV: Sampled Value
// APC: Analogue Process Control

func (n *LogicalNode) CreateDataObjectCDC_ENS(name string) *DataObject {
	return &DataObject{
		object: C.CDC_ENS_create(C.CString(name), (*C.ModelNode)(n.node), 0),
	}
}

func (n *LogicalNode) CreateDataObjectCDC_VSS(name string) *DataObject {
	return &DataObject{
		object: C.CDC_VSS_create(C.CString(name), (*C.ModelNode)(n.node), 0),
	}
}

func (n *LogicalNode) CreateDataObjectCDC_SAV(name string, isInteger bool) *DataObject {
	return &DataObject{
		object: C.CDC_SAV_create(C.CString(name), (*C.ModelNode)(n.node), 0, C.bool(isInteger)),
	}
}

func (n *LogicalNode) CreateDataObjectCDC_APC(name string, ctlModel int) *DataObject {
	return &DataObject{
		object: C.CDC_APC_create(C.CString(name), (*C.ModelNode)(n.node), 0, C.uint(ctlModel), C.bool(false)),
	}
}

type DataAttribute struct {
	attribute *C.DataAttribute
}

func (do *DataObject) GetChild(name string) *DataAttribute {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	return &DataAttribute{
		attribute: (*C.DataAttribute)(unsafe.Pointer(C.ModelNode_getChild((*C.ModelNode)(unsafe.Pointer(do.object)), cName))),
	}
}

// SetTriggerOptions configures which events on this DataAttribute trigger an RCB report.
// Must be called BEFORE starting the server.
// Use TRG_OPT_DATA_CHANGED (1) to fire reports when the value changes,
// TRG_OPT_DATA_UPDATE (4) to fire on every IedServer_updateXxx call regardless of change.
// Example: attr.SetTriggerOptions(iec61850.TRG_OPT_DATA_CHANGED)
func (da *DataAttribute) SetTriggerOptions(opts uint8) {
	da.attribute.triggerOptions = C.uint8_t(opts)
}

type DataSet struct {
	dataSet *C.DataSet
}

// CreateDataSet creates a new DataSet under this LogicalNode.
func (ln *LogicalNode) CreateDataSet(name string) *DataSet {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	cDataSet := C.DataSet_create(cName, ln.node)
	return &DataSet{dataSet: cDataSet}
}

// AddDataSetEntry adds a new DataSetEntry to this DataSet.
func (ds *DataSet) AddDataSetEntry(ref string) {
	cRef := C.CString(ref)
	defer C.free(unsafe.Pointer(cRef))

	C.DataSetEntry_create(ds.dataSet, cRef, -1, nil)
}

// ServerRCB is a server-side Report Control Block model node.
// It is owned by the IedModel and must not be freed independently.
type ServerRCB struct {
	rcb *C.ReportControlBlock
}

// CreateReportControlBlock creates an unbuffered (RP) or buffered (BR) Report Control Block
// under this LogicalNode.
//
//   - name:        RCB node name (e.g. "urcb01")
//   - rptId:       report identifier sent to clients (may be "" to default to name)
//   - buffered:    true → buffered (BR), false → unbuffered (RP)
//   - dataSetName: local dataset name the RCB monitors (e.g. "DS1")
//   - confRef:     configuration revision number
//   - trgOps:      OR of TRG_OPT_* constants (e.g. TRG_OPT_DATA_CHANGED|TRG_OPT_INTEGRITY)
//   - rptOpts:     OR of RPT_OPT_* constants controlling optional fields in reports
//   - bufTm:       buffer time in ms (0 = disabled)
//   - intgPd:      integrity period in ms (0 = disabled)
func (n *LogicalNode) CreateReportControlBlock(
	name, rptId string, buffered bool, dataSetName string,
	confRef uint32, trgOps, rptOpts uint8, bufTm, intgPd uint32,
) *ServerRCB {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	cRptId := C.CString(rptId)
	defer C.free(unsafe.Pointer(cRptId))
	cDataSet := C.CString(dataSetName)
	defer C.free(unsafe.Pointer(cDataSet))

	return &ServerRCB{
		rcb: C.ReportControlBlock_create(
			cName, n.node, cRptId, C.bool(buffered), cDataSet,
			C.uint32_t(confRef), C.uint8_t(trgOps), C.uint8_t(rptOpts),
			C.uint32_t(bufTm), C.uint32_t(intgPd),
		),
	}
}
