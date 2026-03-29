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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SoftwareTaskSpec defines the desired state of SoftwareTask.
type SoftwareTaskSpec struct {
	// repo is the HTTPS URL of the git repository.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https://.*`
	Repo string `json:"repo"`

	// branch is the base branch to clone and work from.
	// +kubebuilder:default="main"
	// +optional
	Branch string `json:"branch,omitempty"`

	// task is the natural-language description of the work to perform.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Task string `json:"task"`

	// agent selects the agent runtime to use.
	// +kubebuilder:default="claude-code"
	// +kubebuilder:validation:Enum=claude-code;codex;open-code
	// +optional
	Agent string `json:"agent,omitempty"`

	// credentials references the Kubernetes Secret and/or inline tokens for authentication.
	// Provide either secretRef (pointing to a Secret with GITHUB_TOKEN and the agent API key),
	// or githubToken (inline) with the agent API key in the Secret, or both.
	// +kubebuilder:validation:Required
	Credentials CredentialsSpec `json:"credentials"`

	// resources defines resource limits for the sandbox pod.
	// +optional
	Resources *SandboxResources `json:"resources,omitempty"`

	// gitAuthorName is the name used for git commits made by the agent.
	// +optional
	GitAuthorName string `json:"gitAuthorName,omitempty"`

	// gitAuthorEmail is the email used for git commits made by the agent.
	// +optional
	GitAuthorEmail string `json:"gitAuthorEmail,omitempty"`

	// maxRetries is the maximum number of retry attempts on failure.
	// +kubebuilder:default=1
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
	// +optional
	MaxRetries *int32 `json:"maxRetries,omitempty"`

	// skills is a list of Skill resource names to load into the agent sandbox.
	// Skills provide files, environment variables, and init commands.
	// +optional
	Skills []string `json:"skills,omitempty"`

	// mcpServers defines task-specific MCP servers made available to the agent.
	// These are merged with any MCP servers defined in referenced skills.
	// +optional
	MCPServers []MCPServer `json:"mcpServers,omitempty"`

	// callbackURL is an optional URL that receives a POST when the task completes or fails.
	// The payload includes taskName, status, pullRequestURL, and message.
	// +optional
	CallbackURL string `json:"callbackURL,omitempty"`
}

// CredentialsSpec references a Kubernetes Secret and/or inline tokens for credential injection.
type CredentialsSpec struct {
	// secretRef is the name of the Secret in the same namespace.
	// Required if githubToken is not provided (the secret must contain GITHUB_TOKEN).
	// Always required for the agent API key (ANTHROPIC_API_KEY or OPENAI_API_KEY).
	// +optional
	SecretRef string `json:"secretRef,omitempty"`

	// githubToken is an inline GitHub token for this specific task.
	// If provided, this takes precedence over GITHUB_TOKEN in the referenced secret.
	// +optional
	GithubToken string `json:"githubToken,omitempty"`
}

// SandboxResources defines the compute resources for the sandbox pod.
type SandboxResources struct {
	// cpu limit for the sandbox pod (e.g., "2").
	// +kubebuilder:default="2"
	// +optional
	CPU resource.Quantity `json:"cpu,omitempty"`

	// memory limit for the sandbox pod (e.g., "4Gi").
	// +kubebuilder:default="4Gi"
	// +optional
	Memory resource.Quantity `json:"memory,omitempty"`

	// timeoutMinutes is the maximum execution time before the task is killed.
	// +kubebuilder:default=30
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=120
	// +optional
	TimeoutMinutes *int32 `json:"timeoutMinutes,omitempty"`
}

// TaskPhase represents the lifecycle phase of a SoftwareTask.
// +kubebuilder:validation:Enum=Pending;Running;Completed;Failed
type TaskPhase string

const (
	TaskPhasePending   TaskPhase = "Pending"
	TaskPhaseRunning   TaskPhase = "Running"
	TaskPhaseCompleted TaskPhase = "Completed"
	TaskPhaseFailed    TaskPhase = "Failed"
)

// SoftwareTaskStatus defines the observed state of SoftwareTask.
type SoftwareTaskStatus struct {
	// phase is the current lifecycle phase of the task.
	// +kubebuilder:default="Pending"
	Phase TaskPhase `json:"phase,omitempty"`

	// podName is the name of the sandbox pod created for this task.
	// +optional
	PodName string `json:"podName,omitempty"`

	// startTime is when the sandbox pod was created.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// completionTime is when the task finished (success or failure).
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// retryCount tracks how many retries have been attempted.
	RetryCount int32 `json:"retryCount,omitempty"`

	// pullRequestURL is the URL of the created PR, if successful.
	// +optional
	PullRequestURL string `json:"pullRequestURL,omitempty"`

	// message provides human-readable status information.
	// +optional
	Message string `json:"message,omitempty"`

	// conditions represent the latest available observations.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=st
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Agent",type=string,JSONPath=`.spec.agent`
// +kubebuilder:printcolumn:name="Repo",type=string,JSONPath=`.spec.repo`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="PR",type=string,JSONPath=`.status.pullRequestURL`,priority=1

// SoftwareTask is the Schema for the softwaretasks API.
type SoftwareTask struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec SoftwareTaskSpec `json:"spec"`

	// +optional
	Status SoftwareTaskStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// SoftwareTaskList contains a list of SoftwareTask.
type SoftwareTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []SoftwareTask `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SoftwareTask{}, &SoftwareTaskList{})
}
