package grind

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/cpuid/v2"
)

// cpuFeatures are the features that matter for hashing, by the name a
// device lists them under.
var cpuFeatures = []struct {
	name string
	id   cpuid.FeatureID
}{
	{"sha", cpuid.SHA},   // x86 SHA-1/SHA-256 instructions (SHA-NI)
	{"avx2", cpuid.AVX2}, // BLAKE3 and BLAKE2b vector code
	{"avx512f", cpuid.AVX512F},
	{"asimd", cpuid.ASIMD},   // Arm NEON
	{"sha2", cpuid.SHA2},     // Arm SHA-256 instructions
	{"sha3", cpuid.SHA3},     // Arm SHA-3 instructions
	{"sha512", cpuid.SHA512}, // Arm SHA-512 instructions
}

// probeCPU reports the CPU. There is always one.
func probeCPU() []Device {
	d := Device{ID: "cpu", Kind: CPU, Name: strings.TrimSpace(cpuid.CPU.BrandName), Threads: threadsOf()}
	if d.Name == "" {
		d.Name = "CPU"
	}
	for _, f := range cpuFeatures {
		if cpuid.CPU.Supports(f.id) {
			d.Features = append(d.Features, f.name)
		}
	}
	return []Device{d}
}

// sysClass is where Linux lists display devices; a variable for tests.
var sysClass = "/sys/class/drm"

// gpuVendors names PCI vendor ids.
var gpuVendors = map[string]string{"0x10de": "NVIDIA", "0x1002": "AMD", "0x8086": "Intel", "0x13b5": "Arm", "0x5143": "Qualcomm", "0x1af4": "virtio"}

// probeGPUs reports the GPUs Linux lists under /sys/class/drm, where no
// GPU grinder is built in: each with a note, and auto passes them over.
func probeGPUs() []Device {
	sys := sysGPUs()
	for i := range sys {
		sys[i].dev.Note = "found; this build has no GPU grinder for this system"
	}
	return sysDevices(sys)
}

// sysGPU is a GPU the system lists, with its PCI vendor id and address.
type sysGPU struct {
	dev    Device
	vendor string // "0x10de"
	pci    string // "01:00.0", "" when unknown
}

// sysGPUs lists the GPUs Linux lists under /sys/class/drm. Other systems
// list none; the OpenCL probe adds what their drivers list.
func sysGPUs() []sysGPU {
	cards, _ := filepath.Glob(filepath.Join(sysClass, "card[0-9]*"))
	sort.Strings(cards)
	var out []sysGPU
	for _, c := range cards {
		base := filepath.Base(c)
		if strings.Contains(base, "-") { // a connector such as card0-HDMI-A-1
			continue
		}
		vendor := readTrim(filepath.Join(c, "device", "vendor"))
		if vendor == "" {
			continue
		}
		name := gpuVendors[vendor]
		if name == "" {
			name = "vendor " + vendor
		}
		if drv := driverOf(filepath.Join(c, "device", "uevent")); drv != "" {
			name += " (" + drv + ")"
		}
		pci := ""
		if link, err := os.Readlink(filepath.Join(c, "device")); err == nil {
			// .../0000:01:00.0: drop the domain.
			b := filepath.Base(link)
			if k := strings.IndexByte(b, ':'); k >= 0 && strings.Count(b, ":") == 2 {
				pci = strings.ToLower(b[k+1:])
			}
		}
		out = append(out, sysGPU{dev: Device{ID: fmt.Sprintf("gpu:%d", len(out)), Kind: GPU, Name: name}, vendor: vendor, pci: pci})
	}
	return out
}

func sysDevices(sys []sysGPU) []Device {
	out := make([]Device, len(sys))
	for i, s := range sys {
		out[i] = s.dev
	}
	return out
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func driverOf(uevent string) string {
	for _, l := range strings.Split(readTrim(uevent), "\n") {
		if v, ok := strings.CutPrefix(l, "DRIVER="); ok {
			return v
		}
	}
	return ""
}
