// report_demo 在同一进程中启动一个 IEC 61850 服务器和客户端，演示 Report 订阅功能。
//
// 服务器模型：
//
//	IED "DEMO" → LD "MEAS" → LN "LLN0"
//	  DO  TEMP  (SAV, float)
//	  DO  HUMID (SAV, float)
//	  DataSet  DS1  →  TEMP.instMag.f, HUMID.instMag.f
//	  RCB  urcb01 (unbuffered, RP)  →  watches DS1
//	         trigger: DATA_CHANGED | INTEGRITY (5 s)
//
// 运行：
//
//	go run ./cmd/report_demo
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weiheng-tech/go-libiec61850/iec61850"
)

const tcpPort = 10400

func main() {
	// ── 1. 构建服务端模型 ─────────────────────────────────────────────────────
	model := iec61850.NewIedModel("DEMO")
	defer model.Destroy()

	ld := model.CreateLogicalDevice("MEAS")
	lln0 := ld.CreateLogicalNode("LLN0")

	doTemp := lln0.CreateDataObjectCDC_SAV("TEMP", false)
	doHumid := lln0.CreateDataObjectCDC_SAV("HUMID", false)

	// 数据集
	ds := lln0.CreateDataSet("DS1")
	ds.AddDataSetEntry("LLN0$MX$TEMP$instMag$f")
	ds.AddDataSetEntry("LLN0$MX$HUMID$instMag$f")

	// Report Control Block（非缓冲，RP）
	// trgOps:  DATA_CHANGED(1) | INTEGRITY(8) = 9
	// rptOpts: TIME_STAMP(2) | REASON_FOR_INCLUSION(4) | DATA_SET(8) = 14
	lln0.CreateReportControlBlock(
		"urcb01", // RCB 节点名
		"urcb01", // rptId
		false,    // unbuffered
		"DS1",    // 数据集名（本地名）
		1,        // confRef
		iec61850.TRG_OPT_DATA_CHANGED|iec61850.TRG_OPT_INTEGRITY,                                  // trgOps
		iec61850.RPT_OPT_TIME_STAMP|iec61850.RPT_OPT_REASON_FOR_INCLUDE|iec61850.RPT_OPT_DATA_SET, // rptOpts
		0,    // bufTm
		5000, // intgPd 5 秒完整性上报
	)

	// 数据属性句柄（用于服务端更新）
	attrTemp := doTemp.GetChild("instMag.f")
	attrHumid := doHumid.GetChild("instMag.f")

	// CDC_SAV_create 默认把 instMag.f 的 triggerOptions 置为 0，
	// 必须手动开启 DATA_CHANGED，否则 RCB 的 TRG_OPT_DATA_CHANGED 不会触发。
	attrTemp.SetTriggerOptions(iec61850.TRG_OPT_DATA_CHANGED)
	attrHumid.SetTriggerOptions(iec61850.TRG_OPT_DATA_CHANGED)

	// ── 2. 启动服务器 ────────────────────────────────────────────────────────
	server := iec61850.NewIedServer(model)
	defer server.Destroy()

	server.LockDataModel()
	server.UpdateFloatAttributeValue(attrTemp, 20.0)
	server.UpdateFloatAttributeValue(attrHumid, 50.0)
	server.UnlockDataModel()

	server.Start(tcpPort)
	defer server.Stop()

	fmt.Printf("[server] started on port %d\n", tcpPort)
	fmt.Printf("[server] RCB ref: DEMOMEAS/LLN0.RP.urcb01\n")
	time.Sleep(300 * time.Millisecond) // let server settle

	// ── 3. 客户端连接并订阅 Report ────────────────────────────────────────────
	client := iec61850.NewIedClient(
		iec61850.ConnectTimeout(5*time.Second),
		iec61850.RequestTimeout(5*time.Second),
	)
	if err := client.Connect("localhost", tcpPort); err != nil {
		fmt.Fprintf(os.Stderr, "[client] connect error: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("[client] connected")

	// 收到 report 的回调
	var reportCount int
	handler := func(r *iec61850.Report) {
		reportCount++
		fmt.Printf("\n[report #%d] rcb=%s dataset=%s seq=%d ts=%dms\n",
			reportCount, r.RcbRef, r.DataSetRef, r.SequenceNo, r.Timestamp)
		for i, e := range r.Entries {
			fmt.Printf("  entry[%d] reason=%-2d value=%v\n", i, e.Reason, e.Value)
		}
	}

	rcbRef := "DEMOMEAS/LLN0.RP.urcb01"
	err := client.SubscribeReport(rcbRef, &iec61850.ReportConfig{
		TrgOps: iec61850.TRG_OPT_DATA_CHANGED | iec61850.TRG_OPT_INTEGRITY,
		IntgPd: 5000,
		GI:     true, // 订阅后立即触发一次全量上报
	}, handler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[client] SubscribeReport error: %v\n", err)
		os.Exit(1)
	}
	defer client.UnsubscribeReport(rcbRef)

	fmt.Printf("[client] subscribed to %s\n", rcbRef)

	// ── 4. 服务端循环更新数值 ─────────────────────────────────────────────────
	go func() {
		temp := 20.0
		humid := 50.0
		for {
			time.Sleep(2 * time.Second)
			temp += 0.5
			humid -= 0.3

			server.LockDataModel()
			server.UpdateFloatAttributeValue(attrTemp, float32(temp))
			server.UpdateFloatAttributeValue(attrHumid, float32(humid))
			server.UnlockDataModel()

			fmt.Printf("[server] updated: TEMP=%.1f  HUMID=%.1f\n", temp, humid)
		}
	}()

	// ── 5. 等待 Ctrl+C ───────────────────────────────────────────────────────
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Printf("\n[done] received %d reports total\n", reportCount)
}
