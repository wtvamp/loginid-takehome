package config_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"loginid-takehome/internal/config"
)

// k8sManifestDoc is the minimal shape this test reads out of a
// Kubernetes Deployment manifest — just enough to find each container's
// image, its env var names, and (for APP_MODE specifically) their
// literal values. A generic map[string]any unmarshal would work too, but
// a typed struct makes the fields this test actually depends on explicit
// and greppable.
type k8sManifestDoc struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []struct {
					Image string `yaml:"image"`
					Env   []struct {
						Name  string `yaml:"name"`
						Value string `yaml:"value"` // only ever read for APP_MODE; every other var's value is deliberately never inspected — some are secretKeyRef-sourced with no literal to read at all
					} `yaml:"env"`
				} `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

// parseDeployments reads every "---"-separated YAML document in path and
// returns just the ones that are Kubernetes Deployments — Services,
// NetworkPolicies, and everything else in these multi-document manifest
// files are irrelevant to this test.
func parseDeployments(t *testing.T, path string) []k8sManifestDoc {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var deployments []k8sManifestDoc
	dec := yaml.NewDecoder(f)
	for {
		var doc k8sManifestDoc
		if err := dec.Decode(&doc); err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("decoding a YAML document from %s: %v", path, err)
		}
		if doc.Kind == "Deployment" {
			deployments = append(deployments, doc)
		}
	}
	return deployments
}

// serviceForImage maps a container image reference to the binary name
// config.RequiredAuthEnvVars expects — matched by substring on the image
// path (e.g. ".../api-service:IMAGE_TAG") rather than by Deployment name,
// so this test doesn't need updating if api-service-issuer is ever
// renamed — that Deployment runs the exact same api-service IMAGE, just
// in a different mode, and this test's whole point is checking that
// mode's own env-var requirements are met regardless of what the
// Deployment is called.
func serviceForImage(image string) (service string, recognized bool) {
	switch {
	case strings.Contains(image, "/idp-connector:"):
		return "idp-connector", true
	case strings.Contains(image, "/api-service:"):
		return "api-service", true
	default:
		return "", false
	}
}

// TestManifestsSetRequiredAuthEnvVars is the systemic fix for two
// separate live-review incidents in one day, both the same shape: a
// Deployment's manifest silently omitted an auth env var its own
// running mode required, and nothing caught it until a real request
// exposed the gap in production. This test makes the requirement itself
// — not just each binary's own startup check — enforced in CI: it reads
// config.RequiredAuthEnvVars (the single source of truth cmd/api-service
// and cmd/idp-connector's own startup validation also reads) and
// asserts every real Deployment in deploy/api-service.yaml and
// deploy/manifests.yaml actually sets everything its own mode requires,
// by parsing the manifest text directly — not by trusting that whoever
// last edited a Deployment remembered to keep it in sync.
func TestManifestsSetRequiredAuthEnvVars(t *testing.T) {
	root := repoRoot(t)
	var allDeployments []struct {
		file string
		doc  k8sManifestDoc
	}
	for _, relPath := range []string{
		filepath.Join("deploy", "api-service.yaml"),
		filepath.Join("deploy", "manifests.yaml"),
	} {
		path := filepath.Join(root, relPath)
		for _, doc := range parseDeployments(t, path) {
			allDeployments = append(allDeployments, struct {
				file string
				doc  k8sManifestDoc
			}{relPath, doc})
		}
	}

	checked := 0
	for _, entry := range allDeployments {
		doc := entry.doc
		// This test only has an opinion about a container matching a
		// binary it recognizes — a Deployment could in principle run a
		// sidecar or an unrelated image in the same pod spec, so every
		// container is checked, not just the first, so a required var
		// set on the wrong container in the pod spec doesn't silently
		// pass just because container[0] happened to be something else.
		for _, c := range doc.Spec.Template.Spec.Containers {
			service, recognized := serviceForImage(c.Image)
			if !recognized {
				continue
			}

			envNames := make(map[string]bool, len(c.Env))
			appMode := ""
			for _, e := range c.Env {
				envNames[e.Name] = true
				if e.Name == "APP_MODE" {
					appMode = e.Value
				}
			}
			// A Deployment that never sets APP_MODE at all runs in
			// verifier mode in production exactly as much as one that
			// sets it explicitly (config.Load()'s own documented
			// default) — this test's required-var lookup needs to match
			// that real behavior, not assume every Deployment states its
			// mode explicitly.
			mode := appMode
			if mode == "" {
				mode = "verifier"
			}

			required := config.RequiredAuthEnvVars(service, mode)
			if len(required) == 0 {
				continue
			}
			checked++

			var missing []string
			for _, name := range required {
				if !envNames[name] {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				t.Errorf("%s: Deployment %q, container %q (service=%s, mode=%s) is missing required env var(s): %v",
					entry.file, doc.Metadata.Name, c.Image, service, mode, missing)
			}
		}
	}

	if checked == 0 {
		t.Fatalf("no recognized Deployment (api-service or idp-connector image) was found in deploy/api-service.yaml or deploy/manifests.yaml — this test's own matching logic may have drifted from the real manifests, silently checking nothing")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	// this file: <repoRoot>/internal/config/manifest_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}
