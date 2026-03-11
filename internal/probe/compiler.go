package probe

import (
	"fmt"
	"os/exec"
	"strings"
)

// DetectSystemCompiler returns the exact package identifier of the system C
// compiler, using the package manager appropriate for the ABI.
//
// This is called by the probe script running on the actual AMI. The result
// is stored in BaseCapabilities.SystemCompiler and later copied to
// LayerManifest.BootstrapCompiler for Tier 0 bootstrap builds.
//
// Output format by ABI:
//
//	linux-gnu-2.34 (rpm):  "gcc-11.4.1-2.amzn2023.0.1.x86_64"
//	linux-gnu-2.35 (dpkg): "gcc-11-11.4.0-1ubuntu1~22.04-amd64"
func DetectSystemCompiler(abi string) (string, error) {
	switch abi {
	case "linux-gnu-2.34":
		return detectRPMCompiler()
	case "linux-gnu-2.35":
		return detectDpkgCompiler()
	default:
		return "", fmt.Errorf("DetectSystemCompiler: unknown abi %q — supported: linux-gnu-2.34, linux-gnu-2.35", abi)
	}
}

// detectRPMCompiler queries the system gcc package via rpm on RHEL-family
// systems (AL2023, Rocky 9/10, RHEL 8/9).
func detectRPMCompiler() (string, error) {
	out, err := exec.Command("rpm", "-q", "gcc",
		"--queryformat", "%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH}").Output()
	if err != nil {
		return "", fmt.Errorf("rpm query for gcc: %w", err)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", fmt.Errorf("rpm query returned empty result — is gcc installed?")
	}
	return result, nil
}

// detectDpkgCompiler queries the system gcc package via dpkg on Debian-family
// systems (Ubuntu 24.04, Debian 12).
func detectDpkgCompiler() (string, error) {
	// gcc is a meta-package; query the versioned package (gcc-11, gcc-12, etc.)
	// by asking dpkg what provides /usr/bin/gcc first.
	whichOut, err := exec.Command("dpkg", "-S", "/usr/bin/gcc").Output()
	if err != nil {
		return "", fmt.Errorf("dpkg -S /usr/bin/gcc: %w", err)
	}
	// output: "gcc-11: /usr/bin/gcc"
	pkg := strings.TrimSpace(strings.Split(string(whichOut), ":")[0])

	out, err := exec.Command("dpkg-query", "-W",
		"-f=${Package}-${Version}-${Architecture}", pkg).Output()
	if err != nil {
		return "", fmt.Errorf("dpkg-query for %s: %w", pkg, err)
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "", fmt.Errorf("dpkg-query returned empty result for %q", pkg)
	}
	return result, nil
}
