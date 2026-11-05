package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"iotbench/internal/deployment"
)

// deploymentCommand is the offline M2 entry point. It reads only the explicit
// descriptor/inventory/observer documents, writes only into the new -out
// directory, and performs no Docker, SUT, lifecycle, load or live probe.
func deploymentCommand(args []string) {
	fs := flag.NewFlagSet("deployment-validate", flag.ExitOnError)
	descriptorPath := fs.String("descriptor", "", "versioned deployment descriptor JSON (required)")
	inventoryPath := fs.String("inventory", "", "caller-supplied observed inventory JSON (required)")
	observerFake := fs.String("observer-fake", "", "optional fake read-only observer inventory JSON; no live observer is implemented")
	evidencePath := fs.String("evidence-manifest", "", "optional caller-supplied evidence manifest to verify and preserve")
	out := fs.String("out", "", "NEW output directory outside input documents (required)")
	_ = fs.Parse(args)
	if *descriptorPath == "" || *inventoryPath == "" {
		die(2, "-descriptor and -inventory are required")
	}
	if *out == "" {
		die(2, "-out must name a new output directory")
	}

	descriptorBytes, err := os.ReadFile(*descriptorPath)
	if err != nil {
		die(2, "%v", err)
	}
	descriptor, err := deployment.DecodeDescriptor(descriptorBytes)
	if err != nil {
		die(2, "%v", err)
	}
	inventoryBytes, err := os.ReadFile(*inventoryPath)
	if err != nil {
		die(2, "%v", err)
	}
	inventory, err := deployment.DecodeInventory(inventoryBytes)
	if err != nil {
		die(2, "%v", err)
	}
	observer := deployment.ObserverReport{}
	observerDoc := (*deployment.EvidenceDocument)(nil)
	if *observerFake != "" {
		fake := deployment.FakeObserver{Path: *observerFake}
		observed, err := fake.Observe(context.Background())
		if err != nil {
			die(2, "%v", err)
		}
		fakeBytes, err := os.ReadFile(*observerFake)
		if err != nil {
			die(2, "%v", err)
		}
		obsCanonical, err := observed.CanonicalSHA256()
		if err != nil {
			die(2, "%v", err)
		}
		invCanonical, err := inventory.CanonicalSHA256()
		if err != nil {
			die(2, "%v", err)
		}
		observer.Name = "fake-file"
		observer.InventorySHA256 = deployment.SHA256Bytes(fakeBytes)
		if obsCanonical != invCanonical {
			observer.Status = deployment.Fail
			observer.Reason = "fake observer inventory differs from -inventory; the two supplied observations are ambiguous"
		} else {
			observer.Status = deployment.Pass
			observer.Reason = "fake file observer returned the same canonical inventory as -inventory; no live observation occurred"
		}
		doc := deployment.EvidenceFromBytes("observer_inventory", *observerFake, fakeBytes)
		observerDoc = &doc
	}

	manifest, err := deployment.NewEvidenceManifest([]deployment.EvidenceDocument{
		deployment.EvidenceFromBytes("descriptor", *descriptorPath, descriptorBytes),
		deployment.EvidenceFromBytes("inventory", *inventoryPath, inventoryBytes),
	})
	if err != nil {
		die(2, "%v", err)
	}
	if observerDoc != nil {
		manifest.Documents = append(manifest.Documents, *observerDoc)
	}
	if *evidencePath != "" {
		b, err := os.ReadFile(*evidencePath)
		if err != nil {
			die(2, "%v", err)
		}
		supplied, err := deployment.DecodeEvidenceManifest(b)
		if err != nil {
			die(2, "%v", err)
		}
		if _, err := supplied.Verify(""); err != nil {
			die(2, "%v", err)
		}
		if err := requireManifestDocument(*supplied, "descriptor", *descriptorPath, deployment.SHA256Bytes(descriptorBytes)); err != nil {
			die(2, "%v", err)
		}
		if err := requireManifestDocument(*supplied, "inventory", *inventoryPath, deployment.SHA256Bytes(inventoryBytes)); err != nil {
			die(2, "%v", err)
		}
		if *observerFake != "" {
			if err := requireManifestDocument(*supplied, "observer_inventory", *observerFake, deployment.SHA256Bytes(mustRead(*observerFake))); err != nil {
				die(2, "%v", err)
			}
		}
		manifest = supplied
	}

	descCanonical, err := descriptor.CanonicalSHA256()
	if err != nil {
		die(2, "%v", err)
	}
	invCanonical, err := inventory.CanonicalSHA256()
	if err != nil {
		die(2, "%v", err)
	}
	report, err := deployment.Validate(*descriptor, *inventory, deployment.ValidateOptions{
		DescriptorSHA256:          deployment.SHA256Bytes(descriptorBytes),
		DescriptorCanonicalSHA256: descCanonical,
		InventorySHA256:           deployment.SHA256Bytes(inventoryBytes),
		InventoryCanonicalSHA256:  invCanonical,
		Observer:                  observer,
		Evidence:                  manifest,
	})
	if err != nil {
		die(2, "%v", err)
	}
	inputs := []string{*descriptorPath, *inventoryPath}
	if *observerFake != "" {
		inputs = append(inputs, *observerFake)
	}
	if *evidencePath != "" {
		inputs = append(inputs, *evidencePath)
	}
	if err := deployment.Write(*out, report, manifest, inputs); err != nil {
		die(2, "%v", err)
	}
	fmt.Println(*out)
	if report.Attestation != deployment.Pass {
		die(1, "deployment attestation: %s", report.Attestation)
	}
}

func requireManifestDocument(m deployment.EvidenceManifest, role, path, hash string) error {
	for _, doc := range m.Documents {
		if doc.Role == role && samePath(doc.Path, path) {
			if doc.SHA256 != hash {
				return fmt.Errorf("evidence manifest %s SHA-256 does not match %s", role, path)
			}
			return nil
		}
	}
	return fmt.Errorf("evidence manifest does not declare %s document %s", role, path)
}

func samePath(a, b string) bool {
	aa, err := filepath.Abs(a)
	if err != nil {
		aa = a
	}
	bb, err := filepath.Abs(b)
	if err != nil {
		bb = b
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
}

func mustRead(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		die(2, "%v", err)
	}
	return b
}
