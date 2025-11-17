package iec61850

// this file is used to import all the packages that are needed include cgo files
// if you want to use the cgo files, you should import this file

import (
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/common/inc"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/goose"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/hal/inc"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/iec61850/inc"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/iec61850/inc_private"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/logging"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/mms/inc"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/mms/inc_private"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/inc/mms/iso_mms/asn1c"

	_ "github.com/weiheng-tech/go-libiec61850/iec61850/lib/linux64"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/lib/linux_armv7l"
	_ "github.com/weiheng-tech/go-libiec61850/iec61850/lib/win64"
)
