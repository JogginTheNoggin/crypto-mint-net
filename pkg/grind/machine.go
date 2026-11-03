package grind

import (
	"net"
	"os"
	"runtime"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// ThisMachine describes this machine for an import's fit check
// (nodepolicy.Fit, D155): its threads, the devices r detects, its
// addresses and its files.
func ThisMachine(r *Registry) nodepolicy.Machine {
	m := nodepolicy.Machine{CPUs: runtime.NumCPU(), Exists: func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}}
	for _, d := range r.Detect() {
		m.Devices = append(m.Devices, d.ID)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok {
				m.Addrs = append(m.Addrs, n.IP)
			}
		}
	}
	return m
}
