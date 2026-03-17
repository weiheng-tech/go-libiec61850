package iec61850

import "github.com/weiheng-tech/go-libiec61850/iec61850/scl_xml"

// SchemaLeaf describes how to locate one named leaf value within a dataset read result.
type SchemaLeaf struct {
	Path    string
	FCDAIdx int // index of the top-level FCDA entry in the dataset
	DAIdx   int // index within the FCDA's sub-structure; -1 if FCDA itself is a leaf
}

// DataSetSchema is a pre-computed path→index mapping built from a DataSetDetail.
// Build once at startup with BuildDataSetSchema; reuse for every read cycle.
// Using a schema eliminates string concatenation and type lookups from the hot path.
type DataSetSchema struct {
	Leaves []SchemaLeaf
	// FCDACount is the expected number of top-level FCDA entries.
	FCDACount int
}

// BuildDataSetSchema pre-computes the flat list of (path, index) pairs for a dataset.
// For FCDA entries with a DAName, one leaf is produced. For DO-level FCDA entries
// (no DAName), one leaf per DA filtered by the FCDA's FC is produced.
func BuildDataSetSchema(detail *scl_xml.DataSetDetail) *DataSetSchema {
	schema := &DataSetSchema{FCDACount: len(detail.FCDA)}

	for idx, fcda := range detail.FCDA {
		base := detail.IEDName + fcda.LDInst + "/" +
			fcda.Prefix + fcda.LNClass + fcda.LNInst + "." + fcda.DOName

		if fcda.DAName != "" {
			// DA-level FCDA: the dataset element is a direct leaf value.
			schema.Leaves = append(schema.Leaves, SchemaLeaf{
				Path:    base + "." + fcda.DAName,
				FCDAIdx: idx,
				DAIdx:   -1,
			})
			continue
		}

		// DO-level FCDA: the dataset element is a structure whose sub-elements
		// correspond to the DO's DAs filtered by the FCDA's FC, in definition order.
		doType := detail.GetDOType(fcda.Prefix, fcda.LNClass, fcda.DOName)
		subIdx := 0
		for _, da := range doType.DA {
			if da.FC != fcda.FC {
				continue
			}
			schema.Leaves = append(schema.Leaves, SchemaLeaf{
				Path:    base + "." + da.Name,
				FCDAIdx: idx,
				DAIdx:   subIdx,
			})
			subIdx++
		}
	}

	return schema
}

// ApplySchema maps raw dataset values to named paths using a pre-computed schema.
// out is not cleared before use — the caller may reuse the same map across calls.
//
// For single-element sub-structures (e.g. SAV instMag which wraps one float),
// one level of unwrapping is applied automatically.
func ApplySchema(schema *DataSetSchema, values []GoMmsValue, out map[string]interface{}) {
	for i := range schema.Leaves {
		leaf := &schema.Leaves[i]
		if leaf.FCDAIdx >= len(values) {
			continue
		}
		topVal := values[leaf.FCDAIdx]

		if leaf.DAIdx < 0 {
			out[leaf.Path] = topVal.Value
			continue
		}

		subList, ok := topVal.Value.([]GoMmsValue)
		if !ok || leaf.DAIdx >= len(subList) {
			continue
		}
		sub := subList[leaf.DAIdx]

		// Unwrap single-element structures (e.g. instMag → f/i).
		if inner, ok := sub.Value.([]GoMmsValue); ok && len(inner) == 1 {
			out[leaf.Path] = inner[0].Value
		} else {
			out[leaf.Path] = sub.Value
		}
	}
}
