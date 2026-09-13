package build

import (
	"strings"
	"testing"
)

// Independent copies of the values fetched from cosign's cosign_checksums.txt for
// v3.0.5. If the pinned map drifts — a typo, a bad paste — these fail rather than
// the build instance trusting the wrong digest.
const (
	cosignV305AMD64 = "db15cc99e6e4837daabab023742aaddc3841ce57f193d11b7c3e06c8003642b2"
	cosignV305ARM64 = "d098f3168ae4b3aa70b4ca78947329b953272b487727d1722cb3cb098a1a20ab"
)

func TestCosignReleaseDigest(t *testing.T) {
	if got, err := cosignReleaseDigest("v3.0.5", "amd64"); err != nil || got != cosignV305AMD64 {
		t.Errorf("v3.0.5/amd64 = %q, %v; want %q, nil", got, err, cosignV305AMD64)
	}
	if got, err := cosignReleaseDigest("v3.0.5", "arm64"); err != nil || got != cosignV305ARM64 {
		t.Errorf("v3.0.5/arm64 = %q, %v; want %q, nil", got, err, cosignV305ARM64)
	}
	// An unpinned version or arch must error, never silently install cosign
	// unverified — there is no trusted digest to check the download against.
	if _, err := cosignReleaseDigest("v9.9.9", "amd64"); err == nil {
		t.Error("unpinned version must error")
	}
	if _, err := cosignReleaseDigest("v3.0.5", "riscv64"); err == nil {
		t.Error("unpinned arch must error")
	}
}

func TestEC2Runner_BuildUserData_VerifiesCosignBeforeUse(t *testing.T) {
	recipe := &Recipe{Dir: "/tmp/recipe", Meta: RecipeMeta{Name: "openmpi", Version: "5.0.6", ABI: "linux-gnu-2.34"}}
	job := &Job{Base: stubBaseRef("al2023", "x86_64"), RegistryURL: "s3://strata-registry"}
	cfg := EC2Config{BucketURL: "s3://strata-registry", BinaryArch: "amd64", Region: "us-east-1"}
	runner := newEC2RunnerWithAPIs(cfg, &fakeEC2{instanceID: "i-test"}, &fakeS3Put{})

	ud, err := runner.buildUserData("job-1", recipe, job)
	if err != nil {
		t.Fatalf("buildUserData: %v", err)
	}

	want := `echo "` + cosignV305AMD64 + `  /usr/local/bin/cosign" | sha256sum -c -`
	if !strings.Contains(ud, want) {
		t.Fatalf("user-data does not verify cosign against its pinned digest; want line:\n  %s", want)
	}

	// The digest check must precede chmod +x: a mismatch has to abort before the
	// binary is made executable and run.
	verifyAt := strings.Index(ud, "sha256sum -c -")
	chmodAt := strings.Index(ud, "chmod +x /usr/local/bin/cosign")
	if verifyAt < 0 || chmodAt < 0 || verifyAt > chmodAt {
		t.Errorf("cosign digest check (idx %d) must come before chmod +x (idx %d)", verifyAt, chmodAt)
	}
}

func TestEC2Runner_BuildUserData_RefusesUnpinnedCosign(t *testing.T) {
	recipe := &Recipe{Dir: "/tmp/recipe", Meta: RecipeMeta{Name: "openmpi", Version: "5.0.6", ABI: "linux-gnu-2.34"}}
	job := &Job{Base: stubBaseRef("al2023", "x86_64"), RegistryURL: "s3://strata-registry"}
	cfg := EC2Config{BucketURL: "s3://strata-registry", BinaryArch: "amd64", Region: "us-east-1", CosignVersion: "v9.9.9-nope"}
	runner := newEC2RunnerWithAPIs(cfg, &fakeEC2{instanceID: "i-test"}, &fakeS3Put{})

	if _, err := runner.buildUserData("job-1", recipe, job); err == nil {
		t.Error("buildUserData must refuse an unpinned cosign version rather than emit an unverified download")
	}
}
