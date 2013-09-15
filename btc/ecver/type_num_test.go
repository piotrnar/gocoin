package ecver

import (
	"testing"
)


func TestSplitExp(t *testing.T) {
	na := new_num_from_string("8554470195de4678b06ede9f9286545b51ff2d9aa756ce35a39011783563ea60", 16)
	r1, r2 := na.split_exp()
	if !r1.equal(new_num_from_string("3271156f58b59bd7aa542ca6972c1910", 16)) {
		t.Error("R1 mismatch")
	}
	if !r2.equal(new_num_from_string("0a8a5afcb465a43b8277801311860430", 16)) {
		t.Error("R2 mismatch")
	}
}

func TestSplit(t *testing.T) {
	ng := new_num_from_string("b618eba71ec03638693405c75fc1c9abb1a74471baaf1a3a8b9005821491c4b4", 16)
	ng1, ng128 := ng.split(128)
	if !ng1.equal(new_num_from_string("b1a74471baaf1a3a8b9005821491c4b4", 16)) {
		t.Error("NG1 mismatch")
	}
	if !ng128.equal(new_num_from_string("b618eba71ec03638693405c75fc1c9ab", 16)) {
		t.Error("NG128 mismatch")
	}
}
