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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"encoding/json"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	factoryv1alpha1 "github.com/kube-foundry/kube-foundry/api/v1alpha1"
)

const (
	pollInterval = 15 * time.Second
)

// SoftwareTaskReconciler reconciles a SoftwareTask object.
type SoftwareTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	AgentImages           map[string]string
	DefaultCPU            string
	DefaultMemory         string
	DefaultTimeoutMinutes int32
}

// +kubebuilder:rbac:groups=factory.factory.io,resources=softwaretasks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=factory.factory.io,resources=softwaretasks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=factory.factory.io,resources=softwaretasks/finalizers,verbs=update
// +kubebuilder:rbac:groups=factory.factory.io,resources=skills,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *SoftwareTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var task factoryv1alpha1.SoftwareTask
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	switch task.Status.Phase {
	case "", factoryv1alpha1.TaskPhasePending:
		return r.handlePending(ctx, &task)
	case factoryv1alpha1.TaskPhaseRunning:
		return r.handleRunning(ctx, &task)
	case factoryv1alpha1.TaskPhaseCompleted:
		return ctrl.Result{}, nil
	case factoryv1alpha1.TaskPhaseFailed:
		return r.handleFailed(ctx, &task)
	default:
		log.Info("Unknown phase", "phase", task.Status.Phase)
		return ctrl.Result{}, nil
	}
}

func (r *SoftwareTaskReconciler) handlePending(ctx context.Context, task *factoryv1alpha1.SoftwareTask) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Validate credentials: need either inline githubToken or a secret with GITHUB_TOKEN,
	// and always need a secret for the agent API key.
	if task.Spec.Credentials.SecretRef == "" && task.Spec.Credentials.GithubToken == "" {
		task.Status.Message = "credentials must provide either secretRef or githubToken"
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// The agent API key always comes from a secret
	if task.Spec.Credentials.SecretRef == "" {
		task.Status.Message = "secretRef is required for the agent API key (ANTHROPIC_API_KEY or OPENAI_API_KEY)"
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      task.Spec.Credentials.SecretRef,
		Namespace: task.Namespace,
	}
	if err := r.Get(ctx, secretKey, secret); err != nil {
		if errors.IsNotFound(err) {
			task.Status.Message = fmt.Sprintf("Secret %q not found", task.Spec.Credentials.SecretRef)
			if updateErr := r.Status().Update(ctx, task); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Validate that the secret contains the required API key for the agent
	apiKeyName := agentAPIKeyName(task.Spec.Agent)
	if _, ok := secret.Data[apiKeyName]; !ok {
		task.Status.Message = fmt.Sprintf("Secret %q missing required key %q for agent %q", task.Spec.Credentials.SecretRef, apiKeyName, task.Spec.Agent)
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// If no inline githubToken, the secret must contain GITHUB_TOKEN
	if task.Spec.Credentials.GithubToken == "" {
		if _, ok := secret.Data["GITHUB_TOKEN"]; !ok {
			task.Status.Message = fmt.Sprintf("Secret %q missing GITHUB_TOKEN and no inline githubToken provided", task.Spec.Credentials.SecretRef)
			if updateErr := r.Status().Update(ctx, task); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}

	// Resolve referenced skills
	skills, err := r.resolveSkills(ctx, task)
	if err != nil {
		task.Status.Message = fmt.Sprintf("Failed to resolve skills: %v", err)
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Ensure the sandbox service account exists in the task namespace
	if err := r.ensureSandboxServiceAccount(ctx, task.Namespace); err != nil {
		return ctrl.Result{}, err
	}

	// Create the sandbox pod
	pod, err := r.buildSandboxPod(ctx, task, skills)
	if err != nil {
		task.Status.Message = fmt.Sprintf("Failed to build sandbox pod: %v", err)
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if err := controllerutil.SetControllerReference(task, pod, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Create(ctx, pod); err != nil {
		if !errors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
	}

	log.Info("Created sandbox pod", "pod", pod.Name)

	now := metav1.Now()
	task.Status.Phase = factoryv1alpha1.TaskPhaseRunning
	task.Status.PodName = pod.Name
	task.Status.StartTime = &now
	task.Status.Message = "Sandbox pod created, agent starting"
	if err := r.Status().Update(ctx, task); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: pollInterval}, nil
}

func (r *SoftwareTaskReconciler) handleRunning(ctx context.Context, task *factoryv1alpha1.SoftwareTask) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	pod := &corev1.Pod{}
	podKey := types.NamespacedName{
		Name:      task.Status.PodName,
		Namespace: task.Namespace,
	}
	if err := r.Get(ctx, podKey, pod); err != nil {
		if errors.IsNotFound(err) {
			now := metav1.Now()
			task.Status.Phase = factoryv1alpha1.TaskPhaseFailed
			task.Status.CompletionTime = &now
			task.Status.Message = "Sandbox pod was deleted unexpectedly"
			if updateErr := r.Status().Update(ctx, task); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			r.sendCallback(log, task)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check timeout
	if task.Status.StartTime != nil {
		timeout := r.getTimeoutMinutes(task)
		elapsed := time.Since(task.Status.StartTime.Time)
		if elapsed > time.Duration(timeout)*time.Minute {
			_ = r.Delete(ctx, pod)
			now := metav1.Now()
			task.Status.Phase = factoryv1alpha1.TaskPhaseFailed
			task.Status.CompletionTime = &now
			task.Status.Message = fmt.Sprintf("Task timed out after %d minutes", timeout)
			if updateErr := r.Status().Update(ctx, task); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			r.sendCallback(log, task)
			return ctrl.Result{}, nil
		}
	}

	switch pod.Status.Phase {
	case corev1.PodSucceeded:
		now := metav1.Now()
		task.Status.Phase = factoryv1alpha1.TaskPhaseCompleted
		task.Status.CompletionTime = &now
		task.Status.Message = "Task completed successfully"
		task.Status.PullRequestURL = extractTerminationMessage(pod)
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		r.sendCallback(log, task)
		return ctrl.Result{}, nil

	case corev1.PodFailed:
		now := metav1.Now()
		task.Status.Phase = factoryv1alpha1.TaskPhaseFailed
		task.Status.CompletionTime = &now
		task.Status.Message = fmt.Sprintf("Sandbox pod failed: %s", extractTerminationMessage(pod))
		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		r.sendCallback(log, task)
		return ctrl.Result{}, nil

	default:
		return ctrl.Result{RequeueAfter: pollInterval}, nil
	}
}

// CallbackPayload is the JSON body sent to the callback URL when a task finishes.
type CallbackPayload struct {
	TaskName       string `json:"taskName"`
	Status         string `json:"status"`
	PullRequestURL string `json:"pullRequestUrl,omitempty"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
}

// sendCallback POSTs the task result to the callback URL if configured.
func (r *SoftwareTaskReconciler) sendCallback(log logr.Logger, task *factoryv1alpha1.SoftwareTask) {
	if task.Spec.CallbackURL == "" {
		return
	}

	status := "completed"
	if task.Status.Phase == factoryv1alpha1.TaskPhaseFailed {
		status = "failed"
	}

	payload := CallbackPayload{
		TaskName:       task.Name,
		Status:         status,
		PullRequestURL: task.Status.PullRequestURL,
	}
	if status == "failed" {
		payload.ErrorMessage = task.Status.Message
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Error(err, "Failed to marshal callback payload")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(task.Spec.CallbackURL, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Error(err, "Failed to send callback", "url", task.Spec.CallbackURL)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		log.Info("Callback returned non-success status", "url", task.Spec.CallbackURL, "status", resp.StatusCode)
	}
}

func (r *SoftwareTaskReconciler) handleFailed(ctx context.Context, task *factoryv1alpha1.SoftwareTask) (ctrl.Result, error) {
	maxRetries := int32(1)
	if task.Spec.MaxRetries != nil {
		maxRetries = *task.Spec.MaxRetries
	}

	if task.Status.RetryCount < maxRetries {
		if task.Status.PodName != "" {
			oldPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      task.Status.PodName,
					Namespace: task.Namespace,
				},
			}
			_ = r.Delete(ctx, oldPod)
		}

		task.Status.Phase = factoryv1alpha1.TaskPhasePending
		task.Status.RetryCount++
		task.Status.PodName = ""
		task.Status.StartTime = nil
		task.Status.CompletionTime = nil
		task.Status.Message = fmt.Sprintf("Retrying (attempt %d/%d)", task.Status.RetryCount+1, maxRetries+1)
		if err := r.Status().Update(ctx, task); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// resolveSkills fetches all Skill resources referenced by the task.
func (r *SoftwareTaskReconciler) resolveSkills(ctx context.Context, task *factoryv1alpha1.SoftwareTask) ([]factoryv1alpha1.Skill, error) {
	if len(task.Spec.Skills) == 0 {
		return nil, nil
	}

	skills := make([]factoryv1alpha1.Skill, 0, len(task.Spec.Skills))
	for _, name := range task.Spec.Skills {
		var skill factoryv1alpha1.Skill
		key := types.NamespacedName{Name: name, Namespace: task.Namespace}
		if err := r.Get(ctx, key, &skill); err != nil {
			if errors.IsNotFound(err) {
				return nil, fmt.Errorf("skill %q not found", name)
			}
			return nil, fmt.Errorf("failed to get skill %q: %w", name, err)
		}
		skills = append(skills, skill)
	}
	return skills, nil
}

// resolveSkillFiles resolves all file contents for the given skills, fetching ConfigMap data as needed.
func (r *SoftwareTaskReconciler) resolveSkillFiles(ctx context.Context, namespace string, skills []factoryv1alpha1.Skill) ([]skillFileResolved, error) {
	var resolved []skillFileResolved
	for _, skill := range skills {
		for _, f := range skill.Spec.Files {
			content := f.Content
			if f.ConfigMapRef != nil {
				cm := &corev1.ConfigMap{}
				key := types.NamespacedName{Name: f.ConfigMapRef.Name, Namespace: namespace}
				if err := r.Get(ctx, key, cm); err != nil {
					return nil, fmt.Errorf("skill %q: failed to get configmap %q: %w", skill.Name, f.ConfigMapRef.Name, err)
				}
				var ok bool
				content, ok = cm.Data[f.ConfigMapRef.Key]
				if !ok {
					return nil, fmt.Errorf("skill %q: configmap %q missing key %q", skill.Name, f.ConfigMapRef.Name, f.ConfigMapRef.Key)
				}
			}
			resolved = append(resolved, skillFileResolved{Path: f.Path, Content: content})
		}
	}
	return resolved, nil
}

type skillFileResolved struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type skillPromptEntry struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// mcpServerResolved is the JSON structure passed to entrypoints via SKILL_MCP_SERVERS.
// Headers with secretKeyRef values are resolved to plain strings before serialization.
type mcpServerResolved struct {
	Name    string            `json:"name"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// resolveMCPServers resolves a list of MCP server definitions, expanding secret references
// in headers and env vars to plain strings.
func (r *SoftwareTaskReconciler) resolveMCPServers(ctx context.Context, namespace string, mcpList []factoryv1alpha1.MCPServer) ([]mcpServerResolved, error) {
	var servers []mcpServerResolved
	for _, mcp := range mcpList {
		resolved := mcpServerResolved{
			Name:    mcp.Name,
			URL:     mcp.URL,
			Command: mcp.Command,
			Args:    mcp.Args,
		}

		// Resolve headers (may reference secrets)
		if len(mcp.Headers) > 0 {
			resolved.Headers = make(map[string]string, len(mcp.Headers))
			for _, h := range mcp.Headers {
				val := h.Value
				if h.ValueFrom != nil {
					secret := &corev1.Secret{}
					key := types.NamespacedName{Name: h.ValueFrom.Name, Namespace: namespace}
					if err := r.Get(ctx, key, secret); err != nil {
						return nil, fmt.Errorf("mcp server %q: failed to get secret %q for header %q: %w", mcp.Name, h.ValueFrom.Name, h.Name, err)
					}
					data, ok := secret.Data[h.ValueFrom.Key]
					if !ok {
						return nil, fmt.Errorf("mcp server %q: secret %q missing key %q for header %q", mcp.Name, h.ValueFrom.Name, h.ValueFrom.Key, h.Name)
					}
					val = string(data)
				}
				resolved.Headers[h.Name] = val
			}
		}

		// Resolve env vars — MCP server env is process-level (stdio) or context (remote),
		// so we resolve secretKeyRef here rather than relying on Kubernetes injection.
		if len(mcp.Env) > 0 {
			resolved.Env = make(map[string]string, len(mcp.Env))
			for _, e := range mcp.Env {
				if e.Value != "" {
					resolved.Env[e.Name] = e.Value
				} else if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
					secret := &corev1.Secret{}
					key := types.NamespacedName{Name: e.ValueFrom.SecretKeyRef.Name, Namespace: namespace}
					if err := r.Get(ctx, key, secret); err != nil {
						return nil, fmt.Errorf("mcp server %q: failed to get secret %q for env %q: %w", mcp.Name, e.ValueFrom.SecretKeyRef.Name, e.Name, err)
					}
					data, ok := secret.Data[e.ValueFrom.SecretKeyRef.Key]
					if !ok {
						return nil, fmt.Errorf("mcp server %q: secret %q missing key %q for env %q", mcp.Name, e.ValueFrom.SecretKeyRef.Name, e.ValueFrom.SecretKeyRef.Key, e.Name)
					}
					resolved.Env[e.Name] = string(data)
				}
			}
		}

		servers = append(servers, resolved)
	}
	return servers, nil
}

// collectMCPServers gathers MCP server definitions from all skills.
func collectMCPServers(skills []factoryv1alpha1.Skill) []factoryv1alpha1.MCPServer {
	var all []factoryv1alpha1.MCPServer
	for _, skill := range skills {
		all = append(all, skill.Spec.MCPServers...)
	}
	return all
}

func (r *SoftwareTaskReconciler) buildSandboxPod(ctx context.Context, task *factoryv1alpha1.SoftwareTask, skills []factoryv1alpha1.Skill) (*corev1.Pod, error) {
	cpu := resource.MustParse(r.DefaultCPU)
	memory := resource.MustParse(r.DefaultMemory)
	if task.Spec.Resources != nil {
		if !task.Spec.Resources.CPU.IsZero() {
			cpu = task.Spec.Resources.CPU
		}
		if !task.Spec.Resources.Memory.IsZero() {
			memory = task.Spec.Resources.Memory
		}
	}

	timeoutMinutes := r.getTimeoutMinutes(task)
	activeDeadline := int64(timeoutMinutes) * 60
	branchName := fmt.Sprintf("factory/%s", task.Name)
	apiKeyName := agentAPIKeyName(task.Spec.Agent)

	env := []corev1.EnvVar{
		{Name: "TASK_DESCRIPTION", Value: task.Spec.Task},
		{Name: "REPO_URL", Value: task.Spec.Repo},
		{Name: "BASE_BRANCH", Value: task.Spec.Branch},
		{Name: "WORK_BRANCH", Value: branchName},
		{Name: "TASK_NAME", Value: task.Name},
		{Name: "GIT_AUTHOR_NAME", Value: task.Spec.GitAuthorName},
		{Name: "GIT_AUTHOR_EMAIL", Value: task.Spec.GitAuthorEmail},
		{
			Name: apiKeyName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: task.Spec.Credentials.SecretRef,
					},
					Key: apiKeyName,
				},
			},
		},
		githubTokenEnv(task),
	}

	// Resolve skill files, prompts, init commands, and MCP servers
	var skillFiles []skillFileResolved
	var skillPrompts []skillPromptEntry
	var initCommands []string
	if len(skills) > 0 {
		resolved, err := r.resolveSkillFiles(ctx, task.Namespace, skills)
		if err != nil {
			return nil, err
		}
		skillFiles = resolved

		for _, skill := range skills {
			env = append(env, skill.Spec.Env...)
			initCommands = append(initCommands, skill.Spec.Init...)
			for _, p := range skill.Spec.Prompts {
				skillPrompts = append(skillPrompts, skillPromptEntry{Name: p.Name, Content: p.Content})
			}
		}
	}

	// Merge MCP servers from skills and task spec
	allMCP := collectMCPServers(skills)
	allMCP = append(allMCP, task.Spec.MCPServers...)
	mcpServers, err := r.resolveMCPServers(ctx, task.Namespace, allMCP)
	if err != nil {
		return nil, err
	}

	// Encode skill data as JSON env vars for the entrypoint to consume
	if len(skillFiles) > 0 {
		filesJSON, err := json.Marshal(skillFiles)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal skill files: %w", err)
		}
		env = append(env, corev1.EnvVar{Name: "SKILL_FILES", Value: string(filesJSON)})
	}
	if len(skillPrompts) > 0 {
		promptsJSON, err := json.Marshal(skillPrompts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal skill prompts: %w", err)
		}
		env = append(env, corev1.EnvVar{Name: "SKILL_PROMPTS", Value: string(promptsJSON)})
	}
	if len(initCommands) > 0 {
		initJSON, err := json.Marshal(initCommands)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal init commands: %w", err)
		}
		env = append(env, corev1.EnvVar{Name: "SKILL_INIT_COMMANDS", Value: string(initJSON)})
	}
	if len(mcpServers) > 0 {
		mcpJSON, err := json.Marshal(mcpServers)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal MCP servers: %w", err)
		}
		env = append(env, corev1.EnvVar{Name: "SKILL_MCP_SERVERS", Value: string(mcpJSON)})
	}

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sandboxPodName(task),
			Namespace: task.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "kube-foundry",
				"app.kubernetes.io/component":  "sandbox",
				"app.kubernetes.io/managed-by": "kube-foundry-operator",
				"factory.io/task":              task.Name,
				"factory.io/agent":             task.Spec.Agent,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy:                corev1.RestartPolicyNever,
			ActiveDeadlineSeconds:        &activeDeadline,
			ServiceAccountName:           "kube-foundry-sandbox",
			AutomountServiceAccountToken: boolPtr(false),
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: boolPtr(true),
				RunAsUser:    int64Ptr(1000),
				RunAsGroup:   int64Ptr(1000),
				FSGroup:      int64Ptr(1000),
			},
			Containers: []corev1.Container{
				{
					Name:            "agent",
					Image:           r.AgentImages[task.Spec.Agent],
					ImagePullPolicy: corev1.PullIfNotPresent,
					Env:             env,
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:              cpu,
							corev1.ResourceMemory:           memory,
							corev1.ResourceEphemeralStorage: resource.MustParse("10Gi"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "workspace", MountPath: "/workspace"},
						{Name: "tmp", MountPath: "/tmp"},
					},
					TerminationMessagePath:   "/tmp/termination-log",
					TerminationMessagePolicy: corev1.TerminationMessageReadFile,
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "workspace",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: quantityPtr(resource.MustParse("10Gi")),
						},
					},
				},
				{
					Name: "tmp",
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{
							SizeLimit: quantityPtr(resource.MustParse("1Gi")),
						},
					},
				},
			},
		},
	}, nil
}

func (r *SoftwareTaskReconciler) getTimeoutMinutes(task *factoryv1alpha1.SoftwareTask) int32 {
	if task.Spec.Resources != nil && task.Spec.Resources.TimeoutMinutes != nil {
		return *task.Spec.Resources.TimeoutMinutes
	}
	return r.DefaultTimeoutMinutes
}

func sandboxPodName(task *factoryv1alpha1.SoftwareTask) string {
	if task.Status.RetryCount > 0 {
		return fmt.Sprintf("%s-sandbox-%d", task.Name, task.Status.RetryCount)
	}
	return fmt.Sprintf("%s-sandbox", task.Name)
}

func extractTerminationMessage(pod *corev1.Pod) string {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Terminated != nil && cs.State.Terminated.Message != "" {
			msg := strings.TrimSpace(cs.State.Terminated.Message)
			return msg
		}
	}
	return "unknown reason"
}

const sandboxServiceAccountName = "kube-foundry-sandbox"

func (r *SoftwareTaskReconciler) ensureSandboxServiceAccount(ctx context.Context, namespace string) error {
	sa := &corev1.ServiceAccount{}
	key := types.NamespacedName{Name: sandboxServiceAccountName, Namespace: namespace}
	if err := r.Get(ctx, key, sa); err != nil {
		if errors.IsNotFound(err) {
			sa = &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sandboxServiceAccountName,
					Namespace: namespace,
					Labels: map[string]string{
						"app.kubernetes.io/name":       "kube-foundry",
						"app.kubernetes.io/component":  "sandbox",
						"app.kubernetes.io/managed-by": "kube-foundry-operator",
					},
				},
				AutomountServiceAccountToken: boolPtr(false),
			}
			return r.Create(ctx, sa)
		}
		return err
	}
	return nil
}

// githubTokenEnv returns the GITHUB_TOKEN env var, using inline token if provided,
// otherwise falling back to the secretRef.
func githubTokenEnv(task *factoryv1alpha1.SoftwareTask) corev1.EnvVar {
	if task.Spec.Credentials.GithubToken != "" {
		return corev1.EnvVar{
			Name:  "GITHUB_TOKEN",
			Value: task.Spec.Credentials.GithubToken,
		}
	}
	return corev1.EnvVar{
		Name: "GITHUB_TOKEN",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: task.Spec.Credentials.SecretRef,
				},
				Key: "GITHUB_TOKEN",
			},
		},
	}
}

// agentAPIKeyName returns the secret key name for the given agent's API key.
func agentAPIKeyName(agent string) string {
	switch agent {
	case "codex":
		return "OPENAI_API_KEY"
	default:
		return "ANTHROPIC_API_KEY"
	}
}

func boolPtr(b bool) *bool                               { return &b }
func int64Ptr(i int64) *int64                            { return &i }
func quantityPtr(q resource.Quantity) *resource.Quantity { return &q }

// SetupWithManager sets up the controller with the Manager.
func (r *SoftwareTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&factoryv1alpha1.SoftwareTask{}).
		Owns(&corev1.Pod{}).
		Named("softwaretask").
		Complete(r)
}
