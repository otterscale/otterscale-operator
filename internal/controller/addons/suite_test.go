/*
Copyright 2026 The OtterScale Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package addons

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	helmv2 "github.com/fluxcd/helm-controller/api/v2"
	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	addonsv1alpha1 "github.com/otterscale/otterscale-operator/api/addons/v1alpha1"
	// +kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = addonsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = helmv2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = kustomizev1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// +kubebuilder:scaffold:scheme

	By("bootstrapping test environment")
	crdPaths := []string{filepath.Join("..", "..", "..", "config", "crd", "bases")}
	crdPaths = append(crdPaths, fluxCRDPaths()...)
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     crdPaths,
		ErrorIfCRDPathMissing: true,
	}

	// Retrieve the first found binary directory to allow running tests from IDEs
	if getFirstFoundEnvTestBinaryDir() != "" {
		testEnv.BinaryAssetsDirectory = getFirstFoundEnvTestBinaryDir()
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	cancel()
	Eventually(func() error {
		return testEnv.Stop()
	}, time.Minute, time.Second).Should(Succeed())
})

// getFirstFoundEnvTestBinaryDir locates the first binary in the specified path.
// ENVTEST-based tests depend on specific binaries, usually located in paths set by
// controller-runtime. When running tests directly (e.g., via an IDE) without using
// Makefile targets, the 'BinaryAssetsDirectory' must be explicitly configured.
//
// This function streamlines the process by finding the required binaries, similar to
// setting the 'KUBEBUILDER_ASSETS' environment variable. To ensure the binaries are
// properly set up, run 'make setup-envtest' beforehand.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		logf.Log.Error(err, "Failed to read directory", "path", basePath)
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

// fluxCRDPaths resolves the FluxCD CRD YAML directories from the Go module cache.
// The CRDs are shipped in the root FluxCD controller modules (e.g. github.com/fluxcd/helm-controller)
// which share the same version as their /api sub-modules listed in go.mod.
func fluxCRDPaths() []string {
	modCache := goModCache()

	// Each entry maps an /api sub-module (listed in go.mod) to its root module
	// that ships CRD YAML files under config/crd/bases/.
	type fluxMod struct {
		apiModule  string
		rootModule string
	}
	mods := []fluxMod{
		{apiModule: "github.com/fluxcd/helm-controller/api", rootModule: "github.com/fluxcd/helm-controller"},
		{apiModule: "github.com/fluxcd/kustomize-controller/api", rootModule: "github.com/fluxcd/kustomize-controller"},
	}

	var paths []string
	for _, m := range mods {
		version := moduleVersion(m.apiModule)
		if version == "" {
			continue
		}
		crdDir := filepath.Join(modCache, m.rootModule+"@"+version, "config", "crd", "bases")
		if _, err := os.Stat(crdDir); err == nil {
			paths = append(paths, crdDir)
		}
	}
	return paths
}

// moduleVersion returns the version of a Go module dependency using go list.
func moduleVersion(module string) string {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Version}}", module).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// goModCache returns the Go module cache directory.
func goModCache() string {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err == nil {
		if dir := strings.TrimSpace(string(out)); dir != "" {
			return dir
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "go", "pkg", "mod")
}
