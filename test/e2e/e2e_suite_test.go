//go:build e2e
// +build e2e

/*
Copyright 2026.

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

package e2e

import (
	"fmt"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kube-foundry/kube-foundry/test/utils"
)

var (
	// managerImage is the manager image to be built and loaded for testing.
	managerImage = "example.com/kube-foundry:v0.0.1"
	// mockAgentImage is the mock agent image used in e2e tests.
	mockAgentImage = "mock-agent:e2e"
)

// TestE2E runs the e2e test suite to validate the solution in an isolated environment.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting kube-foundry e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager image")
	cmd := exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", managerImage))
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("building the mock agent image")
	cmd = exec.Command("make", "docker-build-mock-agent", fmt.Sprintf("MOCK_AGENT_IMG=%s", mockAgentImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the mock agent image")

	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	By("loading the mock agent image on Kind")
	err = utils.LoadImageToKindClusterWithName(mockAgentImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the mock agent image into Kind")
})

var _ = AfterSuite(func() {
	// No CertManager cleanup needed — we don't install it for these tests.
})
