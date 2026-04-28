package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	dbusService   string          = "org.freedesktop.ScreenSaver"
	dbusPath      dbus.ObjectPath = "/org/freedesktop/ScreenSaver"
	dbusInterface string          = "org.freedesktop.ScreenSaver"
)

type ProcessInfo struct {
	pid  uint64
	comm string
}

func parse(pid uint64) (ProcessInfo, error) {
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ProcessInfo{}, err
	}

	return ProcessInfo{
		pid:  pid,
		comm: strings.TrimSpace(string(comm)),
	}, nil
}

func userId() (uint32, error) {
	currentUser, err := user.Current()
	if err != nil {
		return 0, err
	}

	uid, err := strconv.ParseUint(currentUser.Uid, 10, 64)
	if err != nil {
		return 0, err
	}

	return uint32(uid), nil
}

func processes() ([]ProcessInfo, error) {
	uid, err := userId()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}

	var values []ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		var fileInfo fs.FileInfo
		fileInfo, err = entry.Info()
		if err != nil {
			return nil, err
		}

		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uid {
			continue
		}

		var pid uint64
		pid, err = strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil {
			return nil, err
		}

		var processInfo ProcessInfo
		processInfo, err = parse(pid)
		if err != nil {
			return nil, err
		}

		values = append(values, processInfo)
	}

	return values, nil
}

func inhibit(obj dbus.BusObject, info ProcessInfo) (uint32, error) {
	applicationName := info.comm
	reasonForInhibit := "Running script"

	var cookie uint32
	call := obj.Call(fmt.Sprintf("%s.Inhibit", dbusInterface), 0, applicationName, reasonForInhibit)
	err := call.Store(&cookie)

	return cookie, err
}

func unInhibit(obj dbus.BusObject, cookie uint32) error {

	call := obj.Call(fmt.Sprintf("%s.UnInhibit", dbusInterface), 0, cookie)
	_ = call.Body // Wait for it to complete
	return call.Err
}

func isRunning(pid uint64) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return err == nil
}

func main() {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	obj := conn.Object(dbusService, dbusPath)

	infos, err := processes()
	if err != nil {
		panic(err)
	}

	fmt.Println("Select process:")
	for i, info := range infos {
		fmt.Printf("[%2d] %s (%d)\n", i+1, info.comm, info.pid)
	}

	var input int

	for {
		fmt.Print("> ")
		_, err = fmt.Scanln(&input)
		if err != nil {
			continue
		}

		if input < 1 || input > len(infos) {
			continue
		}

		break
	}

	d, err := time.ParseDuration("1s")
	if err != nil {
		panic(err)
	}

	var cookie uint32
	processInfo := infos[input-1]
	cookie, err = inhibit(obj, processInfo)
	if err != nil {
		panic(err)
	}

	for {
		time.Sleep(d)
		if !isRunning(processInfo.pid) {
			break
		}
	}

	err = unInhibit(obj, cookie)
	if err != nil {
		panic(err)
	}
}
