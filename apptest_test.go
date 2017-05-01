package holochain

import (
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
)

func TestTestStringReplacements(t *testing.T) {
	h, cleanup, _, _ := setupTestChainDir()
	defer cleanup()
	var lastMatches = [3][]string{{"complete match", "1st submatch", "2nd submatch"}}

	Convey("it should replace %dna%", t, func() {
		input := "%dna%"
		output := h.TestStringReplacements(input, "", "", "", &lastMatches)
		So(output, ShouldEqual, h.dnaHash.String())
	})

	Convey("it should replace %m%", t, func() {
		input := "%m1.2%"
		output := h.TestStringReplacements(input, "", "", "", &lastMatches)
		So(output, ShouldEqual, "2nd submatch")
	})

}

func TestTest(t *testing.T) {
	h, cleanup, _, _ := setupTestChainDir()
	cleanup() // run cleanup before to be sure it isn't there
	if os.Getenv("DEBUG") != "1" {
		h.config.Loggers.TestPassed.Enabled = false
		h.config.Loggers.TestFailed.Enabled = false
		h.config.Loggers.TestInfo.Enabled = false
	}
	Convey("it should fail if there's no test data", t, func() {
		err := h.Test()
		So(err[0].Error(), ShouldEqual, "open "+h.rootPath+"/"+ChainTestDir+": no such file or directory")
	})

	h, cleanup, _, _ := setupTestChainDir()
	defer cleanup()
	if os.Getenv("DEBUG") != "1" {
		h.config.Loggers.TestPassed.Enabled = false
		h.config.Loggers.TestFailed.Enabled = false
		h.config.Loggers.TestInfo.Enabled = false
	}
	Convey("it should validate on test data", t, func() {
		err := h.Test()
		So(err, ShouldBeNil)
	})
	Convey("it should reset the database state and thus run correctly twice", t, func() {
		err := h.Test()
		So(err, ShouldBeNil)
	})

	Convey("it should fail the test on incorrect input types", t, func() {
		os.Remove(d + "/.holochain/test/test/test_0.json")
		err := writeFile(d+"/.holochain/test/test", "test_0.json", []byte(`[{"Zome":"zySampleZome","FnName":"addEven","Input":2,"Output":"%h%","Err":""}]`))
		So(err, ShouldBeNil)
		err = h.Test()[0]
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "Input was not an expected type: float64")
	})
	Convey("it should fail the test on incorrect data", t, func() {
		os.Remove(d + "/.holochain/test/test/test_0.json")
		err := writeFile(d+"/.holochain/test/test", "test_0.json", []byte(`[{"Zome":"zySampleZome","FnName":"addEven","Input":"2","Output":"","Err":"bogus error"}]`))
		So(err, ShouldBeNil)
		err = h.Test()[0]
		So(err, ShouldNotBeNil)
		//So(err.Error(), ShouldEqual, "Test: test_0:0\n  Expected Error: bogus error\n  Got: nil\n")
		So(err.Error(), ShouldEqual, "bogus error")
	})
}
