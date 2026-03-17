package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/weiheng-tech/go-libiec61850/iec61850"
	"github.com/weiheng-tech/go-libiec61850/iec61850/scl_xml"
)

// ─── shared benchmark server (started lazily, lives until test binary exits) ───

const schemaTestPort = 10302

var (
	schemaOnce   sync.Once
	schemaServer *iec61850.IedServer
	schemaModel  *iec61850.IedModel

	schemaAttrTemp  *iec61850.DataAttribute
	schemaAttrHumid *iec61850.DataAttribute
	schemaAttrStr   *iec61850.DataAttribute
)

func startSchemaServer(tb testing.TB) bool {
	schemaOnce.Do(func() {
		m := iec61850.NewIedModel("bench")
		ld := m.CreateLogicalDevice("LD0")
		lln0 := ld.CreateLogicalNode("LLN0")

		temp := lln0.CreateDataObjectCDC_SAV("TEMP", false)   // float
		humid := lln0.CreateDataObjectCDC_SAV("HUMID", false) // float
		name := lln0.CreateDataObjectCDC_VSS("NAME")

		ds := lln0.CreateDataSet("DS1")
		// DA-level entries (direct leaf)
		ds.AddDataSetEntry("LLN0$MX$TEMP$instMag$f")
		ds.AddDataSetEntry("LLN0$MX$HUMID$instMag$f")
		ds.AddDataSetEntry("LLN0$ST$NAME$stVal")

		schemaAttrTemp = temp.GetChild("instMag.f")
		schemaAttrHumid = humid.GetChild("instMag.f")
		schemaAttrStr = name.GetChild("stVal")

		srv := iec61850.NewIedServer(m)
		srv.LockDataModel()
		srv.UpdateFloatAttributeValue(schemaAttrTemp, 23.5)
		srv.UpdateFloatAttributeValue(schemaAttrHumid, 60.0)
		srv.UpdateVisibleStringAttributeValue(schemaAttrStr, "sensor-A")
		srv.UnlockDataModel()

		srv.Start(schemaTestPort)

		schemaServer = srv
		schemaModel = m
	})
	return schemaServer != nil
}

// makeDetail builds a DataSetDetail matching the server above (DA-level FCDAs).
func makeDetail() *scl_xml.DataSetDetail {
	return &scl_xml.DataSetDetail{
		IEDName: "bench",
		DataSet: scl_xml.DataSet{
			Name: "DS1",
			FCDA: []scl_xml.FCDAEntry{
				{LDInst: "LD0", LNClass: "LLN0", DOName: "TEMP", DAName: "instMag.f", FC: "MX"},
				{LDInst: "LD0", LNClass: "LLN0", DOName: "HUMID", DAName: "instMag.f", FC: "MX"},
				{LDInst: "LD0", LNClass: "LLN0", DOName: "NAME", DAName: "stVal", FC: "ST"},
			},
		},
	}
}

// ─── unit tests ───────────────────────────────────────────────────────────────

// TestBuildDataSetSchema_Paths verifies that BuildDataSetSchema produces the
// correct full paths for DA-level FCDAs (no type template lookup required).
func TestBuildDataSetSchema_Paths(t *testing.T) {
	detail := makeDetail()
	schema := iec61850.BuildDataSetSchema(detail)

	if len(schema.Leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d", len(schema.Leaves))
	}

	wantPaths := []string{
		"benchLD0/LLN0.TEMP.instMag.f",
		"benchLD0/LLN0.HUMID.instMag.f",
		"benchLD0/LLN0.NAME.stVal",
	}
	for i, leaf := range schema.Leaves {
		if leaf.Path != wantPaths[i] {
			t.Errorf("leaf[%d] path = %q, want %q", i, leaf.Path, wantPaths[i])
		}
		if leaf.FCDAIdx != i {
			t.Errorf("leaf[%d] FCDAIdx = %d, want %d", i, leaf.FCDAIdx, i)
		}
		if leaf.DAIdx != -1 {
			t.Errorf("leaf[%d] DAIdx = %d, want -1 (direct leaf)", i, leaf.DAIdx)
		}
	}
}

// TestApplySchema_Values verifies that ApplySchema correctly maps raw GoMmsValues.
func TestApplySchema_Values(t *testing.T) {
	detail := makeDetail()
	schema := iec61850.BuildDataSetSchema(detail)

	rawValues := []iec61850.GoMmsValue{
		{Type: iec61850.MMS_FLOAT, Value: float64(23.5)},
		{Type: iec61850.MMS_FLOAT, Value: float64(60.0)},
		{Type: iec61850.MMS_VISIBLE_STRING, Value: "sensor-A"},
	}

	out := make(map[string]interface{})
	iec61850.ApplySchema(schema, rawValues, out)

	cases := []struct {
		key  string
		want interface{}
	}{
		{"benchLD0/LLN0.TEMP.instMag.f", float64(23.5)},
		{"benchLD0/LLN0.HUMID.instMag.f", float64(60.0)},
		{"benchLD0/LLN0.NAME.stVal", "sensor-A"},
	}
	for _, c := range cases {
		got, ok := out[c.key]
		if !ok {
			t.Errorf("key %q missing from output", c.key)
			continue
		}
		if got != c.want {
			t.Errorf("key %q = %v, want %v", c.key, got, c.want)
		}
	}
}

// TestReadDataSetFlat_Values starts a real server and verifies ReadDataSetFlat
// returns values matching what was set on the server.
func TestReadDataSetFlat_Values(t *testing.T) {
	if !startSchemaServer(t) {
		t.Fatal("failed to start schema server")
	}
	time.Sleep(200 * time.Millisecond) // let server settle

	client := iec61850.NewIedClient(iec61850.ConnectTimeout(5 * time.Second))
	if err := client.Connect("localhost", schemaTestPort); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer client.Close()

	detail := makeDetail()
	schema := iec61850.BuildDataSetSchema(detail)
	out := make(map[string]interface{})

	if err := client.ReadDataSetFlat("benchLD0/LLN0.DS1", schema, out); err != nil {
		t.Fatalf("ReadDataSetFlat: %v", err)
	}

	if v, ok := out["benchLD0/LLN0.TEMP.instMag.f"]; !ok || v != float64(23.5) {
		t.Errorf("TEMP = %v, want 23.5", v)
	}
	if v, ok := out["benchLD0/LLN0.HUMID.instMag.f"]; !ok || v != float64(60.0) {
		t.Errorf("HUMID = %v, want 60.0", v)
	}
	if v, ok := out["benchLD0/LLN0.NAME.stVal"]; !ok || v != "sensor-A" {
		t.Errorf("NAME = %v, want sensor-A", v)
	}
}

// ─── benchmarks ──────────────────────────────────────────────────────────────

// newBenchClient creates a fresh connected client, failing the benchmark on error.
func newBenchClient(b *testing.B) *iec61850.IedClient {
	b.Helper()
	c := iec61850.NewIedClient(iec61850.ConnectTimeout(5 * time.Second))
	if err := c.Connect("localhost", schemaTestPort); err != nil {
		b.Fatalf("connect: %v", err)
	}
	return c
}

// BenchmarkReadDataSetValues_Legacy measures the original polling path:
// ReadDataSetValues → ExplainDataSetValues (string building every call).
func BenchmarkReadDataSetValues_Legacy(b *testing.B) {
	if !startSchemaServer(b) {
		b.Fatal("failed to start schema server")
	}
	time.Sleep(200 * time.Millisecond)

	client := newBenchClient(b)
	defer client.Close()

	detail := makeDetail()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		values, err := client.ReadDataSetValues("benchLD0/LLN0.DS1", "")
		if err != nil {
			b.Fatal(err)
		}
		out, err := client.ExplainDataSetValues(values, detail)
		if err != nil {
			b.Fatal(err)
		}
		_ = out
	}
}

// BenchmarkReadDataSetValues_WithSchema measures the hybrid path:
// ReadDataSetValues (existing) + ApplySchema (pre-computed, no string building).
func BenchmarkReadDataSetValues_WithSchema(b *testing.B) {
	if !startSchemaServer(b) {
		b.Fatal("failed to start schema server")
	}
	time.Sleep(200 * time.Millisecond)

	client := newBenchClient(b)
	defer client.Close()

	detail := makeDetail()
	schema := iec61850.BuildDataSetSchema(detail) // built once
	out := make(map[string]interface{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		values, err := client.ReadDataSetValues("benchLD0/LLN0.DS1", "")
		if err != nil {
			b.Fatal(err)
		}
		for k := range out {
			delete(out, k)
		}
		iec61850.ApplySchema(schema, values, out)
	}
}

// BenchmarkReadDataSetFlat measures the fully-optimised path:
// ReadDataSetFlat — direct C-land extraction with no intermediate []GoMmsValue.
func BenchmarkReadDataSetFlat(b *testing.B) {
	if !startSchemaServer(b) {
		b.Fatal("failed to start schema server")
	}
	time.Sleep(200 * time.Millisecond)

	client := newBenchClient(b)
	defer client.Close()

	detail := makeDetail()
	schema := iec61850.BuildDataSetSchema(detail) // built once
	out := make(map[string]interface{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for k := range out {
			delete(out, k)
		}
		if err := client.ReadDataSetFlat("benchLD0/LLN0.DS1", schema, out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkReadDataSetFlat_ReuseMap measures ReadDataSetFlat where the caller
// reuses both the map and resets it with a pre-known key list (zero allocations
// for map management).
func BenchmarkReadDataSetFlat_ReuseMap(b *testing.B) {
	if !startSchemaServer(b) {
		b.Fatal("failed to start schema server")
	}
	time.Sleep(200 * time.Millisecond)

	client := newBenchClient(b)
	defer client.Close()

	detail := makeDetail()
	schema := iec61850.BuildDataSetSchema(detail)
	out := make(map[string]interface{}, len(schema.Leaves))

	// Pre-warm: first read so all map keys are already allocated.
	_ = client.ReadDataSetFlat("benchLD0/LLN0.DS1", schema, out)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Overwrite existing keys — no map growth, minimal GC pressure.
		if err := client.ReadDataSetFlat("benchLD0/LLN0.DS1", schema, out); err != nil {
			b.Fatal(err)
		}
	}
}

// ExampleBuildDataSetSchema shows the recommended one-time setup pattern.
func ExampleBuildDataSetSchema() {
	detail := &scl_xml.DataSetDetail{
		IEDName: "IED1",
		DataSet: scl_xml.DataSet{
			FCDA: []scl_xml.FCDAEntry{
				{LDInst: "LD0", LNClass: "LLN0", DOName: "TEMP", DAName: "instMag.f", FC: "MX"},
			},
		},
	}
	schema := iec61850.BuildDataSetSchema(detail)
	fmt.Println(schema.Leaves[0].Path)
	// Output: IED1LD0/LLN0.TEMP.instMag.f
}
