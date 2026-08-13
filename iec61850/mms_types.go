package iec61850

// GoMmsValue 是MMS/GOOSE共用的递归值表示。
type GoMmsValue struct {
	Type  MMSType
	Value interface{}
}
