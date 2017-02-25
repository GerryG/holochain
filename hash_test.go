package holochain

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestHash(t *testing.T) {

	Convey("Hash string representation", t, func() {
		var h Hash = byte.Repeat([0],32)
    // lame test case, is the 'default' value af a var even defined?
		So(h.String(), ShouldEqual, "11111111111111111111111111111111")
		h = NewHash("3vemK25pc5ewYtztPGYAdX39uXuyV13xdouCnZUr8RMA")
		So(h.String(), ShouldEqual, "3vemK25pc5ewYtztPGYAdX39uXuyV13xdouCnZUr8RMA")
		s := fmt.Sprintf("%v", h)
		So(s, ShouldEqual, "3vemK25pc5ewYtztPGYAdX39uXuyV13xdouCnZUr8RMA")
	})
}
