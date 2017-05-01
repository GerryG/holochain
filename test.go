package holochain

import (
	"bytes"
	ic "github.com/libp2p/go-libp2p-crypto"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"os/exec"
	"strconv"
	"time"
)

var Crash bool

func Panix(on string) {
	if Crash {
		panic(on)
	}
}

func testDir(tm time.Time) string {
	return "/tmp/holochain_test" + strconv.FormatInt(tm.Unix(), 10) +
		"." + strconv.Itoa(tm.Nanosecond())
}

func mkTestDirName() string {
	return testDir(time.Now())
}

func setupTestService() (cleanup func(), s *Service) {
	d := mkTestDirName()
	cleanup = func() { cleanupTestDir(d) }
	agent := AgentName("Herbert <h@bert.com>")
	s, err := Init(d+"/"+DefaultDirectoryName, agent)
	s.Settings.DefaultBootstrapServer = "localhost:3142"
	if err != nil {
		panic(err)
	}
	return
}

func genTestChain(n string) (cleanup func(), s *Service, h *Holochain) {
	cleanup, s = setupTestService()
	path := s.Path + "/" + n
	Debug("Service up, GenDev next")
	h, err := s.GenDev(path, "toml")
	Debug("GenDev done")
	if err != nil {
		panic(err)
	}
	return
}

func prepareTestChain(n string) (cleanup func(), s *Service, h *Holochain) {
	var err error
	cleanup, s, h = genTestChain("test")
	defer ErrorHandlerf(err, "Error in prepareTestChain %v ", n)

	if _, err = h.GenChain(); err != nil {
		return
	}
	err = h.Activate()
	return
}

func setupTestDir() (dir string) {
	dir = mkTestDirName()
	err := os.MkdirAll(dir, os.ModePerm)
	defer ErrorHandlerf(err, "Error in setupTestDir %v ", dir)
	return
}

func cleanupTestDir(path string) {
	err := os.RemoveAll(path)
	defer ErrorHandlerf(err, "Error in cleanupTestDir %v ", path)
	return
}

func ShouldLog(log *Logger, message string, fn func()) {
	var buf bytes.Buffer
	w := log.w
	log.w = &buf
	e := log.Enabled
	log.Enabled = true
	fn()
	So(buf.String(), ShouldEqual, message)
	log.Enabled = e
	log.w = w
}

func execCmd(cmd string, args ...string) {
	out, err := exec.Command(cmd, args...).Output()
	if err != nil {
		Debugf("exec error %v Cmd:%v A:%v", err, cmd, args)
	}
	Debug(string(out))
}

func chainTestSetup(dir string) (holo *Holochain, hs HashSpec, key ic.PrivKey, now time.Time) {
	a, _ := NewAgent(IPFS, "agent id")
	key = a.PrivKey()
	hc := Holochain{HashType: HASH_SHA}
	if dir != "" {
		hc.rootPath = dir + "/.holochain"
	}
	Debugf("Use %v for rootPath", hc.rootPath)
	holo = &hc
	hc.PrepareHashType()
	hs = hc.hashSpec
	hc.mkChainDirs()
	return
}

func setupTestChainDir() (holo *Holochain, cleanup func(), key ic.PrivKey, now time.Time) {
	dir := setupTestDir()
	cleanup = func() { cleanupTestDir(dir) }
	holo, _, key, now = chainTestSetup(dir)
	if _, err := NewChainFromFile(holo); err != nil {
		Debugf("Test setup, NewChainFromFile for dir %v failed %v", dir, err)
	}
	return
}
