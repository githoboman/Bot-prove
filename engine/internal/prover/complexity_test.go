package prover

import (
	"math"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Run("returns default configuration", func(t *testing.T) {
		config := DefaultConfig()
		if config.ThresholdTrivial != 1024 {
			t.Errorf("Expected ThresholdTrivial to be 1024, got %d", config.ThresholdTrivial)
		}
		if config.ThresholdLow != 65536 {
			t.Errorf("Expected ThresholdLow to be 65536, got %d", config.ThresholdLow)
		}
		if config.ThresholdMedium != 1048576 {
			t.Errorf("Expected ThresholdMedium to be 1048576, got %d", config.ThresholdMedium)
		}
		if config.ThresholdHigh != 16777216 {
			t.Errorf("Expected ThresholdHigh to be 16777216, got %d", config.ThresholdHigh)
		}
	})
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name          string
		inputSize     int
		outputSize    int
		modelSize     int
		expectedClass ComplexityClass
	}{
		{"trivial case", 100, 100, 100, TRIVIAL},
		{"low case", 1025, 100, 100, LOW},
		{"medium case", 65537, 100, 100, MEDIUM},
		{"high case", 1048577, 100, 100, HIGH},
		{"extreme case", 16777217, 100, 100, EXTREME},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := Classify(tt.inputSize, tt.outputSize, tt.modelSize)
			if metrics.Class != tt.expectedClass {
				t.Errorf("Expected class %v, got %v", tt.expectedClass, metrics.Class)
			}
		})
	}
}

func TestClassifyProof(t *testing.T) {
	t.Run("classifies proof correctly", func(t *testing.T) {
		p := &Proof{
			IH: "010203",
			OH: "040506",
			MH: "070809",
		}
		metrics := ClassifyProof(p)
		if metrics == nil {
			t.Fatal("Expected non-nil metrics")
		}
		if metrics.Class != TRIVIAL {
			t.Errorf("Expected TRIVIAL class, got %v", metrics.Class)
		}
	})
}

func TestEstimateGas(t *testing.T) {
	t.Run("returns zero for nil metrics", func(t *testing.T) {
		gas := EstimateGas(nil)
		if gas != 0 {
			t.Errorf("Expected 0 gas for nil metrics, got %d", gas)
		}
	})

	t.Run("calculates gas correctly", func(t *testing.T) {
		metrics := &ComplexityMetrics{
			InputSize:   100,
			OutputSize:  100,
			ModelSize:   100,
			LeafCount:   3,
			TreeDepth:   2,
			MerkleOps:   5,
			EstGasCSPR:  21000 + 68*300 + 2000*5,
			EstTimeMs:  50 /* 300/1024 == 0, no extra per-KB time at this size */,
			Class:       TRIVIAL,
		}
		gas := EstimateGas(metrics)
		expected := int64(21000 + 68*300 + 2000*5)
		if gas != expected {
			t.Errorf("Expected gas %d, got %d", expected, gas)
		}
	})
}

func TestEstimateBatchGas(t *testing.T) {
	t.Run("returns zero for empty slice", func(t *testing.T) {
		gas := EstimateBatchGas(nil)
		if gas != 0 {
			t.Errorf("Expected 0 gas for nil slice, got %d", gas)
		}
	})

	t.Run("sums gas for multiple proofs", func(t *testing.T) {
		proofs := []*Proof{
			{IH: "01", OH: "02", MH: "03"},
			{IH: "04", OH: "05", MH: "06"},
		}
		gas := EstimateBatchGas(proofs)
		expected := int64(21000+68*6+2000*5) * 2 // total bytes per proof = len("01")+len("02")+len("03") = 6
		if gas != expected {
			t.Errorf("Expected gas %d, got %d", expected, gas)
		}
	})

	t.Run("skips nil proofs", func(t *testing.T) {
		proofs := []*Proof{
			{IH: "01", OH: "02", MH: "03"},
			nil,
			{IH: "04", OH: "05", MH: "06"},
		}
		gas := EstimateBatchGas(proofs)
		expected := int64(21000+68*6+2000*5) * 2 // total bytes per proof = len("01")+len("02")+len("03") = 6
		if gas != expected {
			t.Errorf("Expected gas %d, got %d", expected, gas)
		}
	})
}

func TestSuggestBatchSize(t *testing.T) {
	t.Run("returns zero for zero maxGas", func(t *testing.T) {
		size := SuggestBatchSize(nil, 0)
		if size != 0 {
			t.Errorf("Expected 0 batch size for zero maxGas, got %d", size)
		}
	})

	t.Run("returns zero for negative maxGas", func(t *testing.T) {
		size := SuggestBatchSize(nil, -100)
		if size != 0 {
			t.Errorf("Expected 0 batch size for negative maxGas, got %d", size)
		}
	})

	t.Run("fits single proof within limit", func(t *testing.T) {
		proofs := []*Proof{{IH: "01", OH: "02", MH: "03"}}
		size := SuggestBatchSize(proofs, 35000) // > 31408 (one proof's real EstGasCSPR)
		if size != 1 {
			t.Errorf("Expected batch size 1, got %d", size)
		}
	})

	t.Run("fits multiple proofs within limit", func(t *testing.T) {
		proofs := []*Proof{
			{IH: "01", OH: "02", MH: "03"},
			{IH: "04", OH: "05", MH: "06"},
		}
		size := SuggestBatchSize(proofs, 65000) // > 2*31408
		if size != 2 {
			t.Errorf("Expected batch size 2, got %d", size)
		}
	})

	t.Run("stops when limit exceeded", func(t *testing.T) {
		proofs := []*Proof{
			{IH: "01", OH: "02", MH: "03"},
			{IH: "04", OH: "05", MH: "06"},
			{IH: "07", OH: "08", MH: "09"},
		}
		size := SuggestBatchSize(proofs, 65000) // fits 2 proofs (62816) but not 3 (94224)
		if size != 2 {
			t.Errorf("Expected batch size 2, got %d", size)
		}
	})

	t.Run("skips nil proofs", func(t *testing.T) {
		proofs := []*Proof{
			nil,
			{IH: "01", OH: "02", MH: "03"},
			nil,
			{IH: "04", OH: "05", MH: "06"},
		}
		size := SuggestBatchSize(proofs, 65000) // fits 2 proofs (62816) but not 3 (94224)
		if size != 2 {
			t.Errorf("Expected batch size 2, got %d", size)
		}
	})
}

func TestComplexityDistribution(t *testing.T) {
	t.Run("returns empty map for empty slice", func(t *testing.T) {
		dist := ComplexityDistribution(nil)
		if len(dist) != 0 {
			t.Errorf("Expected empty distribution map, got %v", dist)
		}
	})

	t.Run("counts complexity classes correctly", func(t *testing.T) {
		proofs := []*Proof{
			{IH: strings.Repeat("a", 100), OH: strings.Repeat("a", 100), MH: strings.Repeat("a", 100)}, // TRIVIAL
			{IH: strings.Repeat("a", 1025), OH: strings.Repeat("a", 100), MH: strings.Repeat("a", 100)}, // LOW
			{IH: strings.Repeat("a", 65537), OH: strings.Repeat("a", 100), MH: strings.Repeat("a", 100)}, // MEDIUM
			{IH: strings.Repeat("a", 1048577), OH: strings.Repeat("a", 100), MH: strings.Repeat("a", 100)}, // HIGH
			{IH: strings.Repeat("a", 16777217), OH: strings.Repeat("a", 100), MH: strings.Repeat("a", 100)}, // EXTREME
		}
		dist := ComplexityDistribution(proofs)
		if dist[TRIVIAL] != 1 {
			t.Errorf("Expected 1 TRIVIAL proof, got %d", dist[TRIVIAL])
		}
		if dist[LOW] != 1 {
			t.Errorf("Expected 1 LOW proof, got %d", dist[LOW])
		}
		if dist[MEDIUM] != 1 {
			t.Errorf("Expected 1 MEDIUM proof, got %d", dist[MEDIUM])
		}
		if dist[HIGH] != 1 {
			t.Errorf("Expected 1 HIGH proof, got %d", dist[HIGH])
		}
		if dist[EXTREME] != 1 {
			t.Errorf("Expected 1 EXTREME proof, got %d", dist[EXTREME])
		}
	})

	t.Run("skips nil proofs", func(t *testing.T) {
		proofs := []*Proof{
			nil,
			{IH: strings.Repeat("a", 100), OH: strings.Repeat("a", 100), MH: strings.Repeat("a", 100)},
			nil,
		}
		dist := ComplexityDistribution(proofs)
		if dist[TRIVIAL] != 1 {
			t.Errorf("Expected 1 TRIVIAL proof, got %d", dist[TRIVIAL])
		}
	})
}

func TestComplexityClass_String(t *testing.T) {
	tests := []struct {
		name     string
		class    ComplexityClass
		expected string
	}{
		{"TRIVIAL", TRIVIAL, "TRIVIAL"},
		{"LOW", LOW, "LOW"},
		{"MEDIUM", MEDIUM, "MEDIUM"},
		{"HIGH", HIGH, "HIGH"},
		{"EXTREME", EXTREME, "EXTREME"},
		{"UNKNOWN", ComplexityClass(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := tt.class.String()
			if actual != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestClassifyMetrics(t *testing.T) {
	tests := []struct {
		name          string
		inputSize     int
		outputSize    int
		modelSize     int
		expectedClass ComplexityClass
		expectedLeafs int
		expectedDepth int
		expectedOps   int
	}{
		{"trivial metrics", 100, 100, 100, TRIVIAL, 3, 2, 5},
		{"low metrics", 1025, 100, 100, LOW, 3, 2, 5},
		{"medium metrics", 65537, 100, 100, MEDIUM, 3, 2, 5},
		{"high metrics", 1048577, 100, 100, HIGH, 3, 2, 5},
		{"extreme metrics", 16777217, 100, 100, EXTREME, 3, 2, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := Classify(tt.inputSize, tt.outputSize, tt.modelSize)
			if metrics.Class != tt.expectedClass {
				t.Errorf("Expected class %v, got %v", tt.expectedClass, metrics.Class)
			}
			if metrics.LeafCount != tt.expectedLeafs {
				t.Errorf("Expected %d leafs, got %d", tt.expectedLeafs, metrics.LeafCount)
			}
			if metrics.TreeDepth != tt.expectedDepth {
				t.Errorf("Expected depth %d, got %d", tt.expectedDepth, metrics.TreeDepth)
			}
			if metrics.MerkleOps != tt.expectedOps {
				t.Errorf("Expected %d ops, got %d", tt.expectedOps, metrics.MerkleOps)
			}
		})
	}
}

func TestGasEstimation(t *testing.T) {
	tests := []struct {
		name         string
		inputSize    int
		outputSize   int
		modelSize    int
		expectedGas  int64
		expectedTime int64
	}{
		{"trivial gas", 100, 100, 100, 21000 + 68*300 + 2000*5, 50 /* 300/1024 == 0, no extra per-KB time at this size */},
		{"low gas", 1025, 100, 100, 21000 + 68*1225 + 2000*5, 50 + 10*(1225/1024)},
		{"medium gas", 65537, 100, 100, 21000 + 68*65737 + 2000*5, 50 + 10*(65737/1024)}, // total = 65537+100+100 = 65737
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metrics := Classify(tt.inputSize, tt.outputSize, tt.modelSize)
			if metrics.EstGasCSPR != tt.expectedGas {
				t.Errorf("Expected gas %d, got %d", tt.expectedGas, metrics.EstGasCSPR)
			}
			if metrics.EstTimeMs != tt.expectedTime {
				t.Errorf("Expected time %d, got %d", tt.expectedTime, metrics.EstTimeMs)
			}
		})
	}
}

func TestClassifyEdgeCases(t *testing.T) {
	t.Run("zero sizes", func(t *testing.T) {
		metrics := Classify(0, 0, 0)
		if metrics.Class != TRIVIAL {
			t.Errorf("Expected TRIVIAL class for zero sizes, got %v", metrics.Class)
		}
	})

	t.Run("large sizes", func(t *testing.T) {
		metrics := Classify(1<<30, 1<<30, 1<<30)
		if metrics.Class != EXTREME {
			t.Errorf("Expected EXTREME class for large sizes, got %v", metrics.Class)
		}
	})
}

func TestBatchFunctionsEdgeCases(t *testing.T) {
	t.Run("empty proofs slice", func(t *testing.T) {
		gas := EstimateBatchGas([]*Proof{})
		if gas != 0 {
			t.Errorf("Expected 0 gas for empty slice, got %d", gas)
		}

		size := SuggestBatchSize([]*Proof{}, 100000)
		if size != 0 {
			t.Errorf("Expected 0 batch size for empty slice, got %d", size)
		}

		dist := ComplexityDistribution([]*Proof{})
		if len(dist) != 0 {
			t.Errorf("Expected empty distribution for empty slice, got %v", dist)
		}
	})

	t.Run("single nil proof", func(t *testing.T) {
		gas := EstimateBatchGas([]*Proof{nil})
		if gas != 0 {
			t.Errorf("Expected 0 gas for single nil proof, got %d", gas)
		}

		size := SuggestBatchSize([]*Proof{nil}, 100000)
		if size != 0 {
			t.Errorf("Expected 0 batch size for single nil proof, got %d", size)
		}

		dist := ComplexityDistribution([]*Proof{nil})
		if len(dist) != 0 {
			t.Errorf("Expected empty distribution for single nil proof, got %v", dist)
		}
	})
}

func TestComplexityMetricsFields(t *testing.T) {
	t.Run("verifies all metrics fields", func(t *testing.T) {
		metrics := Classify(100, 200, 300)
		if metrics.InputSize != 100 {
			t.Errorf("Expected InputSize 100, got %d", metrics.InputSize)
		}
		if metrics.OutputSize != 200 {
			t.Errorf("Expected OutputSize 200, got %d", metrics.OutputSize)
		}
		if metrics.ModelSize != 300 {
			t.Errorf("Expected ModelSize 300, got %d", metrics.ModelSize)
		}
		if metrics.LeafCount != 3 {
			t.Errorf("Expected LeafCount 3, got %d", metrics.LeafCount)
		}
		if metrics.TreeDepth != 2 {
			t.Errorf("Expected TreeDepth 2, got %d", metrics.TreeDepth)
		}
		if metrics.MerkleOps != 5 {
			t.Errorf("Expected MerkleOps 5, got %d", metrics.MerkleOps)
		}
	})
}

func TestThresholdBoundaries(t *testing.T) {
	tests := []struct {
		name          string
		totalSize     int
		expectedClass ComplexityClass
	}{
		{"trivial boundary", 1024, TRIVIAL},
		{"low boundary", 65536, LOW},
		{"medium boundary", 1048576, MEDIUM},
		{"high boundary", 16777216, HIGH},
		{"extreme boundary", 16777217, EXTREME},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pass the exact boundary as a single component instead of
			// dividing by 3 (integer division on non-multiples-of-3 values
			// like 16777217 silently shrinks the reconstructed total below
			// the intended boundary and misclassifies it).
			metrics := Classify(tt.totalSize, 0, 0)
			if metrics.Class != tt.expectedClass {
				t.Errorf("Expected class %v for size %d, got %v", tt.expectedClass, tt.totalSize, metrics.Class)
			}
		})
	}
}

func TestTreeDepthCalculation(t *testing.T) {
	tests := []struct {
		name        string
		leafCount   int
		expectedDepth int
	}{
		{"3 leafs", 3, 2},
		{"4 leafs", 4, 2},
		{"5 leafs", 5, 3},
		{"8 leafs", 8, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the tree depth calculation logic
			// We can't directly test it since Classify always uses 3 leafs
			// but we can verify the expected depth matches our calculation
			depth := int(math.Ceil(math.Log2(float64(tt.leafCount))))
			if depth != tt.expectedDepth {
				t.Errorf("Expected depth %d for %d leafs, got %d", tt.expectedDepth, tt.leafCount, depth)
			}
		})
	}
}
