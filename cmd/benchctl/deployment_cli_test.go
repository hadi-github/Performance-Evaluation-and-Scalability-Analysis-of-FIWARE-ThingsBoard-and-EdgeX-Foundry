package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const cliFixtureDescriptorJSON = `{
  "schema_version": 1,
  "platform": "thingsboard",
  "project": "tb",
  "allocation": {"application_cpusets": ["4-7"], "supporting_cpusets": ["1", "2-3"]},
  "protocol": {
    "binding_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "profile_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  },
  "provenance": {"source": "synthetic-cli-fixture"},
  "services": [
    {"service": "app", "class": "application", "replicas": 1, "cpuset": "4", "memory_bytes": 1048576},
    {"service": "support", "class": "supporting", "replicas": 1, "cpuset": "1", "memory_bytes": 1048576}
  ]
}`

const cliFixtureInventoryJSON = `{
  "schema_version": 1,
  "project": "tb",
  "source": "synthetic-cli-fixture",
  "services": [
    {
      "project": "tb",
      "service": "app",
      "replicas": 1,
      "cpuset": "4",
      "containers": [
        {"project": "tb", "service": "app", "container_id": "app-0", "replica": 0, "image": "example/app:1", "started_at_ms": 1, "health": "healthy", "state": "running"}
      ]
    },
    {
      "project": "tb",
      "service": "support",
      "replicas": 1,
      "cpuset": "1",
      "containers": [
        {"project": "tb", "service": "support", "container_id": "support-0", "replica": 0, "image": "example/support:1", "started_at_ms": 2, "health": "healthy", "state": "running"}
      ]
    }
  ]
}`

func TestDeploymentValidateCLIStrictCaseExactKeys(t *testing.T) {
	bin := buildBenchctlBinary(t)
	base := t.TempDir()
	descriptorPath := writeCLIFixture(t, base, "descriptor.json", cliFixtureDescriptorJSON)
	inventoryPath := writeCLIFixture(t, base, "inventory.json", cliFixtureInventoryJSON)

	t.Run("valid exact keys", func(t *testing.T) {
		out := filepath.Join(base, "out-valid")
		log, code := runBenchctlBinary(t, bin, "deployment-validate",
			"-descriptor", descriptorPath, "-inventory", inventoryPath, "-out", out)
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, log)
		}
		reportBytes, err := os.ReadFile(filepath.Join(out, "deployment-validation.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report struct {
			Attestation string `json:"attestation"`
			Live        string `json:"live_validation"`
		}
		if err := json.Unmarshal(reportBytes, &report); err != nil {
			t.Fatal(err)
		}
		if report.Attestation != "PASS" || report.Live != "NOT_ASSESSED" {
			t.Fatalf("attestation=%s live=%s, want PASS/NOT_ASSESSED", report.Attestation, report.Live)
		}
	})

	t.Run("descriptor case alias", func(t *testing.T) {
		aliasPath := writeCLIFixture(t, base, "descriptor-platform-alias.json",
			strings.Replace(cliFixtureDescriptorJSON, `"platform"`, `"PLATFORM"`, 1))
		out := filepath.Join(base, "out-descriptor-alias")
		log, code := runBenchctlBinary(t, bin, "deployment-validate",
			"-descriptor", aliasPath, "-inventory", inventoryPath, "-out", out)
		assertCLIRejection(t, log, code, out, `non-case-exact key "PLATFORM"`)
	})

	t.Run("inventory nested case alias", func(t *testing.T) {
		aliasPath := writeCLIFixture(t, base, "inventory-state-alias.json",
			strings.Replace(cliFixtureInventoryJSON, `"state": "running"`, `"STATE": "running"`, 1))
		out := filepath.Join(base, "out-inventory-alias")
		log, code := runBenchctlBinary(t, bin, "deployment-validate",
			"-descriptor", descriptorPath, "-inventory", aliasPath, "-out", out)
		assertCLIRejection(t, log, code, out, `non-case-exact key "STATE"`)
	})

	t.Run("evidence manifest case alias", func(t *testing.T) {
		manifestPath := writeCLIFixture(t, base, "manifest-documents-alias.json",
			strings.Replace(validCLIManifest(t, descriptorPath, inventoryPath), `"documents"`, `"DOCUMENTS"`, 1))
		out := filepath.Join(base, "out-manifest-alias")
		log, code := runBenchctlBinary(t, bin, "deployment-validate",
			"-descriptor", descriptorPath, "-inventory", inventoryPath,
			"-evidence-manifest", manifestPath, "-out", out)
		assertCLIRejection(t, log, code, out, `non-case-exact key "DOCUMENTS"`)
	})

	t.Run("valid evidence manifest", func(t *testing.T) {
		manifestPath := writeCLIFixture(t, base, "manifest-valid.json",
			validCLIManifest(t, descriptorPath, inventoryPath))
		out := filepath.Join(base, "out-manifest-valid")
		log, code := runBenchctlBinary(t, bin, "deployment-validate",
			"-descriptor", descriptorPath, "-inventory", inventoryPath,
			"-evidence-manifest", manifestPath, "-out", out)
		if code != 0 {
			t.Fatalf("exit %d, want 0\n%s", code, log)
		}
		if _, err := os.Stat(filepath.Join(out, "deployment-validation.json")); err != nil {
			t.Fatal(err)
		}
	})
}

func validCLIManifest(t *testing.T, descriptorPath, inventoryPath string) string {
	t.Helper()
	descriptorBytes, err := os.ReadFile(descriptorPath)
	if err != nil {
		t.Fatal(err)
	}
	inventoryBytes, err := os.ReadFile(inventoryPath)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf(`{
  "schema_version": 1,
  "interpretation": "deployment-evidence-manifest-v1",
  "documents": [
    {"role": "descriptor", "path": %q, "sha256": %q},
    {"role": "inventory", "path": %q, "sha256": %q}
  ]
}`, descriptorPath, cliSHA256(descriptorBytes), inventoryPath, cliSHA256(inventoryBytes))
}

func cliSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func buildBenchctlBinary(t *testing.T) string {
	t.Helper()
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "benchctl")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = packageDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build benchctl: %v\n%s", err, out)
	}
	return bin
}

func writeCLIFixture(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func runBenchctlBinary(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return string(out), exitErr.ExitCode()
	}
	t.Fatalf("run benchctl: %v\n%s", err, out)
	return "", -1
}

func assertCLIRejection(t *testing.T, log string, code int, out, want string) {
	t.Helper()
	if code != 2 {
		t.Fatalf("exit %d, want 2 (input rejection)\n%s", code, log)
	}
	if !strings.Contains(log, want) {
		t.Fatalf("log does not contain %q\n%s", want, log)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output directory %s exists after rejected input", out)
	}
}
