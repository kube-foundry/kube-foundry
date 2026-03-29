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

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	factoryv1alpha1 "github.com/kube-foundry/kube-foundry/api/v1alpha1"
)

func newReconciler() *SoftwareTaskReconciler {
	return &SoftwareTaskReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
		AgentImages: map[string]string{
			"claude-code": "ghcr.io/kube-foundry/kube-foundry/agent-claude-code:latest",
			"codex":       "ghcr.io/kube-foundry/kube-foundry/agent-codex:latest",
			"open-code":   "ghcr.io/kube-foundry/kube-foundry/agent-open-code:latest",
		},
		DefaultCPU:            "2",
		DefaultMemory:         "4Gi",
		DefaultTimeoutMinutes: 30,
	}
}

func int32Ptr(i int32) *int32 { return &i }

func createTask(ctx context.Context, name, secretRef string) *factoryv1alpha1.SoftwareTask {
	task := &factoryv1alpha1.SoftwareTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: factoryv1alpha1.SoftwareTaskSpec{
			Repo:   "https://github.com/test/repo",
			Branch: "main",
			Task:   "Add a test file",
			Agent:  "claude-code",
			Credentials: factoryv1alpha1.CredentialsSpec{
				SecretRef: secretRef,
			},
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, task)).To(Succeed())
	return task
}

func reconcileTask(ctx context.Context, name string) (reconcile.Result, error) {
	return newReconciler().Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: name, Namespace: "default"},
	})
}

func cleanupTask(ctx context.Context, name string) {
	task := &factoryv1alpha1.SoftwareTask{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, task); err == nil {
		_ = k8sClient.Delete(ctx, task)
	}
	podList := &corev1.PodList{}
	_ = k8sClient.List(ctx, podList)
	for i := range podList.Items {
		_ = k8sClient.Delete(ctx, &podList.Items[i])
	}
}

var _ = Describe("SoftwareTask Controller", func() {
	const (
		namespace = "default"
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
	)

	ctx := context.Background()

	Context("When reconciling a new SoftwareTask", func() {
		const taskName = "test-task"

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-creds",
					Namespace: namespace,
				},
				StringData: map[string]string{
					"ANTHROPIC_API_KEY": "sk-test",
					"GITHUB_TOKEN":      "ghp_test",
				},
			}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: namespace}, &corev1.Secret{})
			if errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			}
		})

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should create a sandbox pod and transition to Running", func() {
			createTask(ctx, taskName, "test-creds")

			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(pollInterval))

			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      taskName + "-sandbox",
					Namespace: namespace,
				}, pod)
			}, timeout, interval).Should(Succeed())

			Expect(pod.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
			Expect(pod.Spec.Containers).To(HaveLen(1))
			Expect(pod.Spec.Containers[0].Name).To(Equal("agent"))
			Expect(*pod.Spec.AutomountServiceAccountToken).To(BeFalse())

			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhaseRunning))
			Expect(updatedTask.Status.PodName).To(Equal(taskName + "-sandbox"))
			Expect(updatedTask.Status.StartTime).NotTo(BeNil())
		})

		It("should report missing secret", func() {
			createTask(ctx, taskName, "nonexistent-secret")

			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(10 * time.Second))

			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Message).To(ContainSubstring("not found"))
		})
	})

	Context("When a running task's pod succeeds", func() {
		const taskName = "test-succeed"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should transition to Completed with PR URL", func() {
			task := createTask(ctx, taskName, "test-creds")

			// First reconcile: Pending -> Running
			_, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Simulate pod success with termination message
			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: taskName + "-sandbox", Namespace: namespace,
				}, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodSucceeded
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "agent",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 0,
							Message:  "https://github.com/test/repo/pull/99",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			// Second reconcile: Running -> Completed
			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			_ = task
			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhaseCompleted))
			Expect(updatedTask.Status.PullRequestURL).To(Equal("https://github.com/test/repo/pull/99"))
			Expect(updatedTask.Status.CompletionTime).NotTo(BeNil())
			Expect(updatedTask.Status.Message).To(Equal("Task completed successfully"))
		})
	})

	Context("When a running task's pod fails", func() {
		const taskName = "test-fail"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should transition to Failed with error message", func() {
			createTask(ctx, taskName, "test-creds")

			// Pending -> Running
			_, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: taskName + "-sandbox", Namespace: namespace,
				}, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodFailed
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "agent",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Message:  "agent crashed",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			// Running -> Failed
			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhaseFailed))
			Expect(updatedTask.Status.Message).To(ContainSubstring("agent crashed"))
			Expect(updatedTask.Status.CompletionTime).NotTo(BeNil())
		})
	})

	Context("When a running task's pod is deleted unexpectedly", func() {
		const taskName = "test-deleted-pod"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should transition to Failed", func() {
			createTask(ctx, taskName, "test-creds")

			// Pending -> Running
			_, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Delete the pod to simulate unexpected deletion
			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: taskName + "-sandbox", Namespace: namespace,
				}, pod)
			}, timeout, interval).Should(Succeed())
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())

			// Wait for pod to be gone
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name: taskName + "-sandbox", Namespace: namespace,
				}, &corev1.Pod{})
				return errors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())

			// Running -> Failed (pod deleted)
			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhaseFailed))
			Expect(updatedTask.Status.Message).To(ContainSubstring("deleted unexpectedly"))
		})
	})

	Context("When a running task times out", func() {
		const taskName = "test-timeout"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should transition to Failed with timeout message", func() {
			task := createTask(ctx, taskName, "test-creds")

			// Pending -> Running
			_, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Manually set the startTime to be in the past (beyond timeout)
			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			pastTime := metav1.NewTime(time.Now().Add(-31 * time.Minute))
			updatedTask.Status.StartTime = &pastTime
			Expect(k8sClient.Status().Update(ctx, updatedTask)).To(Succeed())

			_ = task
			// Running -> Failed (timeout)
			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			finalTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, finalTask)).To(Succeed())
			Expect(finalTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhaseFailed))
			Expect(finalTask.Status.Message).To(ContainSubstring("timed out"))
			Expect(finalTask.Status.CompletionTime).NotTo(BeNil())
		})
	})

	Context("When a failed task has retries remaining", func() {
		const taskName = "test-retry"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should retry by transitioning back to Pending", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskName,
					Namespace: namespace,
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add a test file",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "test-creds",
					},
					MaxRetries: int32Ptr(2),
				},
			}
			Expect(k8sClient.Create(ctx, task)).To(Succeed())

			// Pending -> Running
			_, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Simulate pod failure
			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: taskName + "-sandbox", Namespace: namespace,
				}, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodFailed
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "agent",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Message:  "error",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			// Running -> Failed
			_, err = reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Failed -> Pending (retry)
			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeTrue())

			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhasePending))
			Expect(updatedTask.Status.RetryCount).To(Equal(int32(1)))
			Expect(updatedTask.Status.Message).To(ContainSubstring("Retrying"))
		})
	})

	Context("When a failed task has exhausted retries", func() {
		const taskName = "test-exhausted"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should stay Failed", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      taskName,
					Namespace: namespace,
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add a test file",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "test-creds",
					},
					MaxRetries: int32Ptr(0),
				},
			}
			Expect(k8sClient.Create(ctx, task)).To(Succeed())

			// Pending -> Running
			_, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Simulate pod failure
			pod := &corev1.Pod{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: taskName + "-sandbox", Namespace: namespace,
				}, pod)
			}, timeout, interval).Should(Succeed())

			pod.Status.Phase = corev1.PodFailed
			pod.Status.ContainerStatuses = []corev1.ContainerStatus{
				{
					Name: "agent",
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1,
							Message:  "error",
						},
					},
				},
			}
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			// Running -> Failed
			_, err = reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())

			// Failed with maxRetries=0 -> stays Failed
			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Requeue).To(BeFalse())
			Expect(result.RequeueAfter).To(BeZero())

			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			Expect(updatedTask.Status.Phase).To(Equal(factoryv1alpha1.TaskPhaseFailed))
			Expect(updatedTask.Status.RetryCount).To(Equal(int32(0)))
		})
	})

	Context("When a completed task is reconciled", func() {
		const taskName = "test-completed"

		AfterEach(func() {
			cleanupTask(ctx, taskName)
		})

		It("should be a no-op", func() {
			task := createTask(ctx, taskName, "test-creds")

			// Manually set to Completed
			_ = task
			updatedTask := &factoryv1alpha1.SoftwareTask{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: namespace}, updatedTask)).To(Succeed())
			updatedTask.Status.Phase = factoryv1alpha1.TaskPhaseCompleted
			Expect(k8sClient.Status().Update(ctx, updatedTask)).To(Succeed())

			result, err := reconcileTask(ctx, taskName)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(result.Requeue).To(BeFalse())
		})
	})

	Context("When reconciling a deleted task", func() {
		It("should return without error", func() {
			result, err := reconcileTask(ctx, "nonexistent-task")
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(result.Requeue).To(BeFalse())
		})
	})
})

var _ = Describe("SoftwareTask Controller Helpers", func() {
	Describe("sandboxPodName", func() {
		It("should return taskName-sandbox for first attempt", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{Name: "my-task"},
				Status:     factoryv1alpha1.SoftwareTaskStatus{RetryCount: 0},
			}
			Expect(sandboxPodName(task)).To(Equal("my-task-sandbox"))
		})

		It("should include retry count for retries", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{Name: "my-task"},
				Status:     factoryv1alpha1.SoftwareTaskStatus{RetryCount: 1},
			}
			Expect(sandboxPodName(task)).To(Equal("my-task-sandbox-1"))
		})

		It("should include higher retry counts", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{Name: "my-task"},
				Status:     factoryv1alpha1.SoftwareTaskStatus{RetryCount: 3},
			}
			Expect(sandboxPodName(task)).To(Equal("my-task-sandbox-3"))
		})
	})

	Describe("extractTerminationMessage", func() {
		It("should extract termination message from container status", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "agent",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Message: "https://github.com/test/repo/pull/1",
								},
							},
						},
					},
				},
			}
			Expect(extractTerminationMessage(pod)).To(Equal("https://github.com/test/repo/pull/1"))
		})

		It("should trim whitespace from termination message", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "agent",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									Message: "  some message  \n",
								},
							},
						},
					},
				},
			}
			Expect(extractTerminationMessage(pod)).To(Equal("some message"))
		})

		It("should return 'unknown reason' when no termination message", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "agent",
							State: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: 1,
								},
							},
						},
					},
				},
			}
			Expect(extractTerminationMessage(pod)).To(Equal("unknown reason"))
		})

		It("should return 'unknown reason' when no container statuses", func() {
			pod := &corev1.Pod{}
			Expect(extractTerminationMessage(pod)).To(Equal("unknown reason"))
		})

		It("should return 'unknown reason' when container is not terminated", func() {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name: "agent",
							State: corev1.ContainerState{
								Running: &corev1.ContainerStateRunning{},
							},
						},
					},
				},
			}
			Expect(extractTerminationMessage(pod)).To(Equal("unknown reason"))
		})
	})

	Describe("getTimeoutMinutes", func() {
		reconciler := &SoftwareTaskReconciler{
			DefaultTimeoutMinutes: 30,
		}

		It("should return default when no resources specified", func() {
			task := &factoryv1alpha1.SoftwareTask{}
			Expect(reconciler.getTimeoutMinutes(task)).To(Equal(int32(30)))
		})

		It("should return default when resources specified but no timeout", func() {
			task := &factoryv1alpha1.SoftwareTask{
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Resources: &factoryv1alpha1.SandboxResources{
						CPU: resource.MustParse("4"),
					},
				},
			}
			Expect(reconciler.getTimeoutMinutes(task)).To(Equal(int32(30)))
		})

		It("should return custom timeout when specified", func() {
			timeout := int32(60)
			task := &factoryv1alpha1.SoftwareTask{
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Resources: &factoryv1alpha1.SandboxResources{
						TimeoutMinutes: &timeout,
					},
				},
			}
			Expect(reconciler.getTimeoutMinutes(task)).To(Equal(int32(60)))
		})
	})

	Describe("agentAPIKeyName", func() {
		It("should return ANTHROPIC_API_KEY for claude-code", func() {
			Expect(agentAPIKeyName("claude-code")).To(Equal("ANTHROPIC_API_KEY"))
		})

		It("should return OPENAI_API_KEY for codex", func() {
			Expect(agentAPIKeyName("codex")).To(Equal("OPENAI_API_KEY"))
		})

		It("should return ANTHROPIC_API_KEY for open-code", func() {
			Expect(agentAPIKeyName("open-code")).To(Equal("ANTHROPIC_API_KEY"))
		})

		It("should return ANTHROPIC_API_KEY for unknown agents", func() {
			Expect(agentAPIKeyName("unknown")).To(Equal("ANTHROPIC_API_KEY"))
		})
	})

	Describe("buildSandboxPod", func() {
		var reconciler *SoftwareTaskReconciler

		BeforeEach(func() {
			reconciler = &SoftwareTaskReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
				AgentImages: map[string]string{
					"claude-code": "test-agent:latest",
					"codex":       "test-codex:latest",
					"open-code":   "test-opencode:latest",
				},
				DefaultCPU:            "2",
				DefaultMemory:         "4Gi",
				DefaultTimeoutMinutes: 30,
			}
		})

		It("should build a pod with correct structure", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "build-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, nil)
			Expect(err).NotTo(HaveOccurred())

			By("verifying pod metadata")
			Expect(pod.Name).To(Equal("build-test-sandbox"))
			Expect(pod.Namespace).To(Equal("default"))

			By("verifying labels")
			Expect(pod.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "kube-foundry"))
			Expect(pod.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "sandbox"))
			Expect(pod.Labels).To(HaveKeyWithValue("factory.io/task", "build-test"))
			Expect(pod.Labels).To(HaveKeyWithValue("factory.io/agent", "claude-code"))

			By("verifying pod spec")
			Expect(pod.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))
			Expect(pod.Spec.ServiceAccountName).To(Equal("kube-foundry-sandbox"))
			Expect(*pod.Spec.AutomountServiceAccountToken).To(BeFalse())
			Expect(pod.Spec.ActiveDeadlineSeconds).NotTo(BeNil())
			Expect(*pod.Spec.ActiveDeadlineSeconds).To(Equal(int64(30 * 60)))

			By("verifying security context")
			Expect(pod.Spec.SecurityContext).NotTo(BeNil())
			Expect(*pod.Spec.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(*pod.Spec.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(*pod.Spec.SecurityContext.RunAsGroup).To(Equal(int64(1000)))

			By("verifying container")
			Expect(pod.Spec.Containers).To(HaveLen(1))
			container := pod.Spec.Containers[0]
			Expect(container.Name).To(Equal("agent"))
			Expect(container.Image).To(Equal("test-agent:latest"))
			Expect(container.TerminationMessagePath).To(Equal("/tmp/termination-log"))

			By("verifying environment variables")
			envMap := make(map[string]string)
			for _, env := range container.Env {
				if env.Value != "" {
					envMap[env.Name] = env.Value
				}
			}
			Expect(envMap).To(HaveKeyWithValue("TASK_DESCRIPTION", "Add tests"))
			Expect(envMap).To(HaveKeyWithValue("REPO_URL", "https://github.com/test/repo"))
			Expect(envMap).To(HaveKeyWithValue("BASE_BRANCH", "main"))
			Expect(envMap).To(HaveKeyWithValue("WORK_BRANCH", "factory/build-test"))
			Expect(envMap).To(HaveKeyWithValue("TASK_NAME", "build-test"))

			// Verify secret ref env vars exist (claude-code uses ANTHROPIC_API_KEY)
			var hasAPIKey, hasGitHubToken bool
			for _, env := range container.Env {
				if env.Name == "ANTHROPIC_API_KEY" && env.ValueFrom != nil {
					hasAPIKey = true
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("my-creds"))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("ANTHROPIC_API_KEY"))
				}
				if env.Name == "GITHUB_TOKEN" && env.ValueFrom != nil {
					hasGitHubToken = true
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("my-creds"))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("GITHUB_TOKEN"))
				}
			}
			Expect(hasAPIKey).To(BeTrue(), "ANTHROPIC_API_KEY env var should reference secret")
			Expect(hasGitHubToken).To(BeTrue(), "GITHUB_TOKEN env var should reference secret")

			By("verifying volumes")
			Expect(pod.Spec.Volumes).To(HaveLen(2))
			volumeNames := []string{pod.Spec.Volumes[0].Name, pod.Spec.Volumes[1].Name}
			Expect(volumeNames).To(ContainElements("workspace", "tmp"))

			By("verifying volume mounts")
			Expect(container.VolumeMounts).To(HaveLen(2))
			mountPaths := make(map[string]string)
			for _, vm := range container.VolumeMounts {
				mountPaths[vm.Name] = vm.MountPath
			}
			Expect(mountPaths).To(HaveKeyWithValue("workspace", "/workspace"))
			Expect(mountPaths).To(HaveKeyWithValue("tmp", "/tmp"))

			By("verifying resource limits")
			Expect(container.Resources.Limits.Cpu().String()).To(Equal("2"))
			Expect(container.Resources.Limits.Memory().String()).To(Equal("4Gi"))
		})

		It("should use custom resources when specified", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-resources",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Build",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "creds",
					},
					Resources: &factoryv1alpha1.SandboxResources{
						CPU:            resource.MustParse("4"),
						Memory:         resource.MustParse("8Gi"),
						TimeoutMinutes: int32Ptr(60),
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(pod.Spec.Containers[0].Resources.Limits.Cpu().String()).To(Equal("4"))
			Expect(pod.Spec.Containers[0].Resources.Limits.Memory().String()).To(Equal("8Gi"))
			Expect(*pod.Spec.ActiveDeadlineSeconds).To(Equal(int64(60 * 60)))
		})

		It("should use codex image and OPENAI_API_KEY for codex agent", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "codex-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "codex",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(pod.Spec.Containers[0].Image).To(Equal("test-codex:latest"))
			Expect(pod.Labels).To(HaveKeyWithValue("factory.io/agent", "codex"))

			var hasOpenAIKey bool
			for _, env := range pod.Spec.Containers[0].Env {
				if env.Name == "OPENAI_API_KEY" && env.ValueFrom != nil {
					hasOpenAIKey = true
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("my-creds"))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("OPENAI_API_KEY"))
				}
				// Ensure ANTHROPIC_API_KEY is NOT set for codex
				Expect(env.Name).NotTo(Equal("ANTHROPIC_API_KEY"))
			}
			Expect(hasOpenAIKey).To(BeTrue(), "OPENAI_API_KEY env var should reference secret for codex")
		})

		It("should use open-code image and ANTHROPIC_API_KEY for open-code agent", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "opencode-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "open-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(pod.Spec.Containers[0].Image).To(Equal("test-opencode:latest"))
			Expect(pod.Labels).To(HaveKeyWithValue("factory.io/agent", "open-code"))

			var hasAnthropicKey bool
			for _, env := range pod.Spec.Containers[0].Env {
				if env.Name == "ANTHROPIC_API_KEY" && env.ValueFrom != nil {
					hasAnthropicKey = true
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("my-creds"))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("ANTHROPIC_API_KEY"))
				}
			}
			Expect(hasAnthropicKey).To(BeTrue(), "ANTHROPIC_API_KEY env var should reference secret for open-code")
		})

		It("should use retry-based pod name when retryCount > 0", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "retry-pod",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Build",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "creds",
					},
				},
				Status: factoryv1alpha1.SoftwareTaskStatus{
					RetryCount: 2,
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(pod.Name).To(Equal("retry-pod-sandbox-2"))
		})

		It("should inject skill env vars and file data", func() {
			skills := []factoryv1alpha1.Skill{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "test-skill"},
					Spec: factoryv1alpha1.SkillSpec{
						Prompts: []factoryv1alpha1.SkillPrompt{
							{Name: "style", Content: "Use table-driven tests."},
							{Name: "errors", Content: "Wrap errors with fmt.Errorf."},
						},
						Files: []factoryv1alpha1.SkillFile{
							{Path: "CLAUDE.md", Content: "You are a Go expert."},
						},
						Env: []corev1.EnvVar{
							{Name: "MY_CUSTOM_VAR", Value: "hello"},
						},
						Init: []string{"echo setup done"},
					},
				},
			}

			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "skill-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
					Skills: []string{"test-skill"},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, skills)
			Expect(err).NotTo(HaveOccurred())

			container := pod.Spec.Containers[0]
			envMap := make(map[string]string)
			for _, env := range container.Env {
				if env.Value != "" {
					envMap[env.Name] = env.Value
				}
			}

			Expect(envMap).To(HaveKey("MY_CUSTOM_VAR"))
			Expect(envMap["MY_CUSTOM_VAR"]).To(Equal("hello"))
			Expect(envMap).To(HaveKey("SKILL_FILES"))
			Expect(envMap["SKILL_FILES"]).To(ContainSubstring("CLAUDE.md"))
			Expect(envMap["SKILL_FILES"]).To(ContainSubstring("You are a Go expert."))
			Expect(envMap).To(HaveKey("SKILL_PROMPTS"))
			Expect(envMap["SKILL_PROMPTS"]).To(ContainSubstring("style"))
			Expect(envMap["SKILL_PROMPTS"]).To(ContainSubstring("Use table-driven tests."))
			Expect(envMap["SKILL_PROMPTS"]).To(ContainSubstring("errors"))
			Expect(envMap["SKILL_PROMPTS"]).To(ContainSubstring("Wrap errors with fmt.Errorf."))
			Expect(envMap).To(HaveKey("SKILL_INIT_COMMANDS"))
			Expect(envMap["SKILL_INIT_COMMANDS"]).To(ContainSubstring("echo setup done"))
		})

		It("should inject MCP server configs", func() {
			skills := []factoryv1alpha1.Skill{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "mcp-skill"},
					Spec: factoryv1alpha1.SkillSpec{
						MCPServers: []factoryv1alpha1.MCPServer{
							{
								Name: "remote-server",
								URL:  "https://mcp.example.com/sse",
								Headers: []factoryv1alpha1.MCPHeader{
									{Name: "Authorization", Value: "Bearer test-token"},
								},
							},
							{
								Name:    "stdio-server",
								Command: "npx",
								Args:    []string{"-y", "@modelcontextprotocol/server-github"},
								Env: []corev1.EnvVar{
									{Name: "GITHUB_TOKEN", Value: "ghp_test123"},
								},
							},
						},
					},
				},
			}

			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mcp-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
					Skills: []string{"mcp-skill"},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, skills)
			Expect(err).NotTo(HaveOccurred())

			container := pod.Spec.Containers[0]
			envMap := make(map[string]string)
			for _, env := range container.Env {
				if env.Value != "" {
					envMap[env.Name] = env.Value
				}
			}

			Expect(envMap).To(HaveKey("SKILL_MCP_SERVERS"))
			mcpJSON := envMap["SKILL_MCP_SERVERS"]

			By("verifying remote server config")
			Expect(mcpJSON).To(ContainSubstring("remote-server"))
			Expect(mcpJSON).To(ContainSubstring("https://mcp.example.com/sse"))
			Expect(mcpJSON).To(ContainSubstring("Authorization"))
			Expect(mcpJSON).To(ContainSubstring("Bearer test-token"))

			By("verifying stdio server config")
			Expect(mcpJSON).To(ContainSubstring("stdio-server"))
			Expect(mcpJSON).To(ContainSubstring("npx"))
			Expect(mcpJSON).To(ContainSubstring("@modelcontextprotocol/server-github"))
			Expect(mcpJSON).To(ContainSubstring("ghp_test123"))
		})

		It("should merge task-level and skill-level MCP servers", func() {
			skills := []factoryv1alpha1.Skill{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "skill-mcp"},
					Spec: factoryv1alpha1.SkillSpec{
						MCPServers: []factoryv1alpha1.MCPServer{
							{
								Name:    "from-skill",
								Command: "npx",
								Args:    []string{"-y", "skill-server"},
							},
						},
					},
				},
			}

			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "merge-mcp-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
					MCPServers: []factoryv1alpha1.MCPServer{
						{
							Name: "from-task",
							URL:  "https://mcp.staging.internal/sse",
						},
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, skills)
			Expect(err).NotTo(HaveOccurred())

			container := pod.Spec.Containers[0]
			var mcpJSON string
			for _, env := range container.Env {
				if env.Name == "SKILL_MCP_SERVERS" {
					mcpJSON = env.Value
				}
			}
			Expect(mcpJSON).NotTo(BeEmpty(), "SKILL_MCP_SERVERS should be set")
			Expect(mcpJSON).To(ContainSubstring("from-skill"))
			Expect(mcpJSON).To(ContainSubstring("skill-server"))
			Expect(mcpJSON).To(ContainSubstring("from-task"))
			Expect(mcpJSON).To(ContainSubstring("https://mcp.staging.internal/sse"))
		})

		It("should set SKILL_MCP_SERVERS from task-level only (no skills)", func() {
			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "task-only-mcp",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
					MCPServers: []factoryv1alpha1.MCPServer{
						{
							Name: "task-server",
							URL:  "https://mcp.example.com/v1",
						},
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, nil)
			Expect(err).NotTo(HaveOccurred())

			container := pod.Spec.Containers[0]
			var mcpJSON string
			for _, env := range container.Env {
				if env.Name == "SKILL_MCP_SERVERS" {
					mcpJSON = env.Value
				}
			}
			Expect(mcpJSON).NotTo(BeEmpty())
			Expect(mcpJSON).To(ContainSubstring("task-server"))
			Expect(mcpJSON).To(ContainSubstring("https://mcp.example.com/v1"))
		})

		It("should not set SKILL_MCP_SERVERS when no MCP servers defined", func() {
			skills := []factoryv1alpha1.Skill{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "no-mcp-skill"},
					Spec: factoryv1alpha1.SkillSpec{
						Files: []factoryv1alpha1.SkillFile{
							{Path: "README.md", Content: "hello"},
						},
					},
				},
			}

			task := &factoryv1alpha1.SoftwareTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-mcp-test",
					Namespace: "default",
				},
				Spec: factoryv1alpha1.SoftwareTaskSpec{
					Repo:   "https://github.com/test/repo",
					Branch: "main",
					Task:   "Add tests",
					Agent:  "claude-code",
					Credentials: factoryv1alpha1.CredentialsSpec{
						SecretRef: "my-creds",
					},
				},
			}

			pod, err := reconciler.buildSandboxPod(ctx, task, skills)
			Expect(err).NotTo(HaveOccurred())

			container := pod.Spec.Containers[0]
			for _, env := range container.Env {
				Expect(env.Name).NotTo(Equal("SKILL_MCP_SERVERS"))
			}
		})
	})

	Describe("helper functions", func() {
		It("boolPtr should return a pointer to the value", func() {
			t := boolPtr(true)
			f := boolPtr(false)
			Expect(*t).To(BeTrue())
			Expect(*f).To(BeFalse())
		})

		It("int64Ptr should return a pointer to the value", func() {
			p := int64Ptr(42)
			Expect(*p).To(Equal(int64(42)))
		})

		It("quantityPtr should return a pointer to the value", func() {
			q := resource.MustParse("10Gi")
			p := quantityPtr(q)
			Expect(p.String()).To(Equal("10Gi"))
		})
	})
})

// Verify the controller properly generates work branch names
var _ = Describe("SoftwareTask work branch naming", func() {
	ctx := context.Background()

	It("should generate factory/<task-name> branch names", func() {
		reconciler := &SoftwareTaskReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
			AgentImages: map[string]string{
				"claude-code": "test:latest",
				"codex":       "test:latest",
				"open-code":   "test:latest",
			},
			DefaultCPU:            "1",
			DefaultMemory:         "1Gi",
			DefaultTimeoutMinutes: 10,
		}
		task := &factoryv1alpha1.SoftwareTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-feature-task",
				Namespace: "default",
			},
			Spec: factoryv1alpha1.SoftwareTaskSpec{
				Repo:   "https://github.com/test/repo",
				Branch: "develop",
				Task:   "Implement feature",
				Agent:  "claude-code",
				Credentials: factoryv1alpha1.CredentialsSpec{
					SecretRef: "creds",
				},
			},
		}

		pod, err := reconciler.buildSandboxPod(ctx, task, nil)
		Expect(err).NotTo(HaveOccurred())
		var workBranch string
		for _, env := range pod.Spec.Containers[0].Env {
			if env.Name == "WORK_BRANCH" {
				workBranch = env.Value
				break
			}
		}
		Expect(workBranch).To(Equal(fmt.Sprintf("factory/%s", task.Name)))
	})
})
