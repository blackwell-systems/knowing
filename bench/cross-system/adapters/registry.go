package adapters

import (
	"os/exec"

	"github.com/blackwell-systems/knowing/bench/cross-system/benchtype"
)

// AvailableAdapter wraps an adapter with an availability check.
type AvailableAdapter struct {
	benchtype.Adapter
	available bool
}

// AllAdapters returns all known adapters with their availability status.
// Only adapters whose dependencies are installed will be usable.
func AllAdapters() []AvailableAdapter {
	return []AvailableAdapter{
		{Adapter: NewKnowing(), available: true}, // always available (it's us)
		{Adapter: NewGrep(), available: hasCommand("rg")},
		{Adapter: NewGitNexus(), available: hasCommand("gitnexus")},
		{Adapter: NewGortex(), available: gortexAvailable()},
		{Adapter: NewAider(), available: aiderAvailable()},
		{Adapter: NewCGC(), available: hasCommand("cgc")},
	}
}

// Available returns only adapters whose dependencies are present on this system.
func Available() []benchtype.Adapter {
	var result []benchtype.Adapter
	for _, a := range AllAdapters() {
		if a.available {
			result = append(result, a.Adapter)
		}
	}
	return result
}

// UnavailableNames returns names of adapters that are NOT available.
func UnavailableNames() []string {
	var result []string
	for _, a := range AllAdapters() {
		if !a.available {
			result = append(result, a.Adapter.Name())
		}
	}
	return result
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func gortexAvailable() bool {
	cmd := exec.Command("/tmp/gortex/gortex", "version")
	return cmd.Run() == nil
}

func aiderAvailable() bool {
	for _, py := range []string{"/tmp/aider-bench/bin/python3", "python3"} {
		cmd := exec.Command(py, "-c", "import aider.repomap")
		if cmd.Run() == nil {
			return true
		}
	}
	return false
}
