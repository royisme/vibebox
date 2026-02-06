package backend

import (
	"context"
	"fmt"
	"runtime"

	"vibebox/internal/config"
)

// Selection describes chosen backend and diagnostics.
type Selection struct {
	Backend      Backend
	Provider     config.Provider
	Diagnostics  map[string]ProbeResult
	WasFallback  bool
	FallbackFrom string
}

func Select(ctx context.Context, provider config.Provider, off, appleVM, docker Backend) (Selection, error) {
	provider = config.NormalizeProvider(provider)
	if err := provider.Validate(); err != nil {
		return Selection{}, err
	}

	diag := map[string]ProbeResult{}
	offProbe := off.Probe(ctx)
	appleProbe := appleVM.Probe(ctx)
	dockerProbe := docker.Probe(ctx)
	diag[off.Name()] = offProbe
	diag[appleVM.Name()] = appleProbe
	diag[docker.Name()] = dockerProbe

	fail := func(target string) error {
		return fmt.Errorf(
			"requested provider %s is unavailable (%s). hints: %v",
			target,
			diag[target].Reason,
			diag[target].FixHints,
		)
	}

	switch provider {
	case config.ProviderOff:
		if !offProbe.Available {
			return Selection{}, fail(off.Name())
		}
		return Selection{Backend: off, Provider: config.ProviderOff, Diagnostics: diag}, nil
	case config.ProviderAppleVM:
		if !appleProbe.Available {
			return Selection{}, fail(appleVM.Name())
		}
		return Selection{Backend: appleVM, Provider: config.ProviderAppleVM, Diagnostics: diag}, nil
	case config.ProviderDocker:
		if !dockerProbe.Available {
			return Selection{}, fail(docker.Name())
		}
		return Selection{Backend: docker, Provider: config.ProviderDocker, Diagnostics: diag}, nil
	case config.ProviderAuto:
		if runtime.GOOS == "darwin" && appleProbe.Available {
			return Selection{Backend: appleVM, Provider: config.ProviderAppleVM, Diagnostics: diag}, nil
		}
		if dockerProbe.Available {
			fallback := runtime.GOOS == "darwin"
			return Selection{
				Backend:      docker,
				Provider:     config.ProviderDocker,
				Diagnostics:  diag,
				WasFallback:  fallback,
				FallbackFrom: "apple-vm",
			}, nil
		}
		return Selection{}, fmt.Errorf(
			"auto selection failed: apple-vm unavailable (%s); docker unavailable (%s)",
			appleProbe.Reason,
			dockerProbe.Reason,
		)
	default:
		return Selection{}, fmt.Errorf("unsupported provider: %s", provider)
	}
}
