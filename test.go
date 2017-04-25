package holochain

import (
	"bytes"
	"fmt"
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

func mkTestDirName() string {
	t := time.Now()
	d := "/tmp/holochain_test" + strconv.FormatInt(t.Unix(), 10) + "." + strconv.Itoa(t.Nanosecond())
	return d
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
	Debug("Service up, GenDev next\n")
	h, err := s.GenDev(path, "toml")
	Debug("GenDev done\n")
	if err != nil {
		panic(err)
	}
	return
}

func prepareTestChain(n string) (cleanup func(), s *Service, h *Holochain) {
	cleanup, s, h = genTestChain("test")
	_, err := h.GenChain()
	if err != nil {
		panic(err)
	}
	err = h.Activate()
	if err != nil {
		panic(err)
	}
	return
}

func setupTestDir() (dir string) {
	dir = mkTestDirName()
	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		panic(err)
	}
	return
}

func cleanupTestDir(path string) {
	err := os.RemoveAll(path)
	if err != nil {
		panic(err)
	}
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
		fmt.Printf("exec error %v Cmd:%v A:%v\n", err, cmd, args)
	}
	fmt.Print(string(out))
}

func chainTestSetup(dir string) (holo *Holochain, hs HashType, key ic.PrivKey, now time.Time) {
	a, _ := NewAgent(IPFS, "agent id")
	key = a.PrivKey()
	hc := Holochain{HashType: HASH_SHA, WireType: WIRE_GOB}
	if dir != "" {
		hc.rootPath = dir + "/.holochain"
	}
	//Debugf("Use %v for rootPath\n", hc.rootPath)
	holo = &hc
	holo.PrepareHashType()
	hs = holo.HashSpec
	holo.mkChainDirs()
	return
}

func setupTestChainDir() (holo *Holochain, cleanup func(), key ic.PrivKey, now time.Time) {
	dir := setupTestDir()
	cleanup = func() { cleanupTestDir(dir) }
	holo, _, key, now = chainTestSetup(dir)
	if err := holo.NewChainFromFile(); err != nil {
		Debugf("Test setup, NewChainFromFile for dir %v failed %v", dir, err)
	}
	return
}
