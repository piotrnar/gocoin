package ecver

import (
	"testing"
)


func TestFeInv(t *testing.T) {
	in := new_fe_from_string("813925AF112AAB8243F8CCBADE4CC7F63DF387263028DE6E679232A73A7F3C31", 16)
	exp := new_fe_from_string("7F586430EA30F914965770F6098E492699C62EE1DF6CAFFA77681C179FDF3117", 16)
	in.inv_s()
	if !in.equal(exp) {
		t.Error("fe.inv() failed")
	}
}
