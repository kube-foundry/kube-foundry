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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kube-foundry/kube-foundry/test/utils"
)

// namespace where the operator is deployed
const namespace = "kube-foundry-system"

// taskNamespace is a separate namespace for SoftwareTask resources
const taskNamespace = "e2e-tasks"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	BeforeAll(func() {
		By("creating manager namespace")
		cmd := exec.Command("kubectl", "create", "ns", namespace)
		_, _ = utils.Run(cmd) // ignore if exists

		By("labeling the namespace to enforce the restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("patching the manager deployment to use the mock agent image")
		cmd = exec.Command("kubectl", "patch", "deployment",
			"kube-foundry-controller-manager",
			"-n", namespace,
			"--type=json",
			"-p", fmt.Sprintf(`[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--agent-image-claude-code=%s"},{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--agent-image-codex=%s"},{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--agent-image-open-code=%s"}]`, mockAgentImage, mockAgentImage, mockAgentImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch manager deployment with mock agent image")

		By("waiting for the manager rollout to complete")
		cmd = exec.Command("kubectl", "rollout", "status", "deployment",
			"kube-foundry-controller-manager",
			"-n", namespace, "--timeout=120s")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Manager rollout did not complete in time")

		By("creating task namespace")
		cmd = exec.Command("kubectl", "create", "ns", taskNamespace)
		_, _ = utils.Run(cmd) // ignore if exists

		By("creating sandbox service account in task namespace")
		cmd = exec.Command("kubectl", "create", "serviceaccount",
			"kube-foundry-sandbox", "-n", taskNamespace)
		_, _ = utils.Run(cmd) // ignore if exists
	})

	AfterAll(func() {
		By("undeploying the controller-manager")
		cmd := exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing task namespace")
		cmd = exec.Command("kubectl", "delete", "ns", taskNamespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("removing manager namespace")
		cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events in task namespace")
			cmd = exec.Command("kubectl", "get", "events", "-n", taskNamespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Operator deployment", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", namespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", namespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})
	})

	Context("SoftwareTask lifecycle", func() {
		It("should complete a task successfully (happy path)", func() {
			taskName := "e2e-happy"

			By("creating the credentials secret")
			cmd := exec.Command("kubectl", "create", "secret", "generic",
				"e2e-creds",
				"--from-literal=ANTHROPIC_API_KEY=fake-key",
				"--from-literal=GITHUB_TOKEN=fake-token",
				"-n", taskNamespace)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create credentials secret")

			By("creating a SoftwareTask with MOCK:success")
			cmd = exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			cmd.Stdin = taskManifest(taskName, "MOCK:success - implement feature X",
				"e2e-creds", nil, nil)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SoftwareTask")

			By("waiting for the sandbox pod to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod",
					fmt.Sprintf("%s-sandbox", taskName),
					"-n", taskNamespace,
					"-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%s-sandbox", taskName)))
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for task to reach Completed phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "softwaretask", taskName,
					"-n", taskNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Completed"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the PR URL is populated")
			cmd = exec.Command("kubectl", "get", "softwaretask", taskName,
				"-n", taskNamespace,
				"-o", "jsonpath={.status.pullRequestURL}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("github.com"))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "softwaretask", taskName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should stay Pending when the credentials secret is missing", func() {
			taskName := "e2e-missing-secret"

			By("creating a SoftwareTask referencing a non-existent secret")
			cmd := exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			cmd.Stdin = taskManifest(taskName, "MOCK:success - should not run",
				"nonexistent-secret", nil, nil)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SoftwareTask")

			By("verifying the task stays Pending with an error message")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "softwaretask", taskName,
					"-n", taskNamespace,
					"-o", "jsonpath={.status.message}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("not found"))
			}, 60*time.Second, 5*time.Second).Should(Succeed())

			By("verifying no sandbox pod was created")
			cmd = exec.Command("kubectl", "get", "pod",
				fmt.Sprintf("%s-sandbox", taskName),
				"-n", taskNamespace)
			_, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Pod should not exist for missing secret")

			By("verifying task phase is still Pending")
			cmd = exec.Command("kubectl", "get", "softwaretask", taskName,
				"-n", taskNamespace,
				"-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeElementOf("Pending", ""))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "softwaretask", taskName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should fail after exhausting retries", func() {
			taskName := "e2e-retry"
			maxRetries := int32(1)

			By("creating a SoftwareTask that will fail with maxRetries=1")
			cmd := exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			cmd.Stdin = taskManifest(taskName, "MOCK:failure - this should fail",
				"e2e-creds", &maxRetries, nil)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SoftwareTask")

			By("waiting for the first sandbox pod to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod",
					fmt.Sprintf("%s-sandbox", taskName),
					"-n", taskNamespace,
					"-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%s-sandbox", taskName)))
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for the retry pod to appear (pod name with -1 suffix)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod",
					fmt.Sprintf("%s-sandbox-1", taskName),
					"-n", taskNamespace,
					"-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(fmt.Sprintf("%s-sandbox-1", taskName)))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for task to reach Failed phase (retries exhausted)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "softwaretask", taskName,
					"-n", taskNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Failed"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying retryCount is 1")
			cmd = exec.Command("kubectl", "get", "softwaretask", taskName,
				"-n", taskNamespace,
				"-o", "jsonpath={.status.retryCount}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1"))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "softwaretask", taskName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should fail with timeout when agent hangs", func() {
			taskName := "e2e-timeout"
			timeoutMinutes := int32(1)

			By("creating a SoftwareTask that will sleep forever with 1-minute timeout")
			cmd := exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			maxRetries := int32(0) // no retries, so it goes directly to Failed
			cmd.Stdin = taskManifest(taskName, "MOCK:timeout - this should hang",
				"e2e-creds", &maxRetries, &timeoutMinutes)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SoftwareTask")

			By("waiting for the sandbox pod to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pod",
					fmt.Sprintf("%s-sandbox", taskName),
					"-n", taskNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for task to reach Failed phase with timeout message")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "softwaretask", taskName,
					"-n", taskNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Failed"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the failure message mentions timeout")
			cmd = exec.Command("kubectl", "get", "softwaretask", taskName,
				"-n", taskNamespace,
				"-o", "jsonpath={.status.message}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("timed out"))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "softwaretask", taskName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should test webhook server integration", func() {
			Skip("Webhook server e2e tests deferred — requires deployment manifests not yet created")
		})
	})

	Context("Skills integration", func() {
		It("should inject skill files, env vars, and init commands into the sandbox pod", func() {
			taskName := "e2e-skill-inject"
			skillName := "e2e-test-skill"

			By("creating a Skill resource")
			cmd := exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			cmd.Stdin = skillManifest(skillName)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create Skill")

			By("creating a SoftwareTask that references the skill")
			cmd = exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			maxRetries := int32(0)
			cmd.Stdin = taskManifestWithSkills(taskName,
				"MOCK:check-skills - verify skill injection",
				"e2e-creds", &maxRetries, nil, []string{skillName})
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SoftwareTask")

			By("waiting for task to reach Completed phase (skills were injected correctly)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "softwaretask", taskName,
					"-n", taskNamespace,
					"-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Completed"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the PR URL is populated (mock agent validated skills)")
			cmd = exec.Command("kubectl", "get", "softwaretask", taskName,
				"-n", taskNamespace,
				"-o", "jsonpath={.status.pullRequestURL}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("github.com"))

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "softwaretask", taskName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "skill", skillName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should fail when referencing a non-existent skill", func() {
			taskName := "e2e-skill-missing"

			By("creating a SoftwareTask that references a non-existent skill")
			cmd := exec.Command("kubectl", "apply", "-n", taskNamespace, "-f", "-")
			maxRetries := int32(0)
			cmd.Stdin = taskManifestWithSkills(taskName,
				"MOCK:success - should not run",
				"e2e-creds", &maxRetries, nil, []string{"nonexistent-skill"})
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SoftwareTask")

			By("verifying the task stays Pending with a skill resolution error")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "softwaretask", taskName,
					"-n", taskNamespace,
					"-o", "jsonpath={.status.message}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("skill"))
				g.Expect(output).To(ContainSubstring("not found"))
			}, 60*time.Second, 5*time.Second).Should(Succeed())

			By("verifying no sandbox pod was created")
			cmd = exec.Command("kubectl", "get", "pod",
				fmt.Sprintf("%s-sandbox", taskName),
				"-n", taskNamespace)
			_, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Pod should not exist when skill is missing")

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "softwaretask", taskName, "-n", taskNamespace, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})

// taskManifest returns a reader with the YAML manifest for a SoftwareTask.
func taskManifest(name, taskDescription, secretRef string, maxRetries *int32, timeoutMinutes *int32) *strings.Reader {
	return taskManifestWithSkills(name, taskDescription, secretRef, maxRetries, timeoutMinutes, nil)
}

// taskManifestWithSkills returns a reader with the YAML manifest for a SoftwareTask, optionally with skills.
func taskManifestWithSkills(name, taskDescription, secretRef string, maxRetries *int32, timeoutMinutes *int32, skills []string) *strings.Reader {
	manifest := fmt.Sprintf(`apiVersion: factory.factory.io/v1alpha1
kind: SoftwareTask
metadata:
  name: %s
spec:
  repo: "https://github.com/example/repo"
  branch: "main"
  task: "%s"
  credentials:
    secretRef: %s`, name, taskDescription, secretRef)

	if maxRetries != nil {
		manifest += fmt.Sprintf("\n  maxRetries: %d", *maxRetries)
	}

	if timeoutMinutes != nil {
		manifest += fmt.Sprintf(`
  resources:
    timeoutMinutes: %d`, *timeoutMinutes)
	}

	if len(skills) > 0 {
		manifest += "\n  skills:"
		for _, s := range skills {
			manifest += fmt.Sprintf("\n    - %s", s)
		}
	}

	manifest += "\n"
	return strings.NewReader(manifest)
}

// skillManifest returns a reader with the YAML manifest for a Skill.
func skillManifest(name string) *strings.Reader {
	manifest := fmt.Sprintf(`apiVersion: factory.factory.io/v1alpha1
kind: Skill
metadata:
  name: %s
spec:
  description: "E2E test skill"
  files:
    - path: CLAUDE.md
      content: "You are a test expert."
  env:
    - name: SKILL_TEST_VAR
      value: "skill-injected"
  init:
    - "echo skill-init-ran"
`, name)
	return strings.NewReader(manifest)
}
