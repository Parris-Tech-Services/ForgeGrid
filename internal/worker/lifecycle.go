package worker

type Lifecycle interface {
	Mode() string
	Start(tx *UpdateTransaction) error
}

// GetLifecycle is a package variable, not a plain function, so tests that
// exercise the update transaction state machine can swap it for a stub via
// t.Cleanup instead of depending on spawning a real OS process - a real
// executable is inherently platform-specific (a Unix shebang script is not
// a valid Windows PE binary), and that distinction is irrelevant to what
// those tests verify.
var GetLifecycle = func(mode string) Lifecycle {
	switch mode {
	case "windows-service":
		return newWindowsServiceLifecycle()
	case "systemd":
		return newSystemdLifecycle()
	default:
		return newPortableLifecycle()
	}
}

func DetectCurrentLifecycle() string {
	return detectLifecycleOS()
}
