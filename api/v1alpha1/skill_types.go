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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SkillPrompt defines a markdown prompt file loaded into the agent's context.
// Each prompt is written to .claude/skills/{name}.md in the workspace,
// matching the Claude Code skills convention.
type SkillPrompt struct {
	// name is the prompt identifier, used as the filename ({name}.md).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// content is the markdown instruction content.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Content string `json:"content"`
}

// SkillSpec defines the desired state of Skill.
type SkillSpec struct {
	// description is a human-readable summary of what this skill provides.
	// +optional
	Description string `json:"description,omitempty"`

	// prompts are markdown instructions loaded into the agent's context as .claude/skills/{name}.md files.
	// This is the primary skill content — it tells the agent how to behave.
	// +optional
	Prompts []SkillPrompt `json:"prompts,omitempty"`

	// files are injected into the agent workspace before the agent starts.
	// +optional
	Files []SkillFile `json:"files,omitempty"`

	// env defines additional environment variables passed to the agent container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// init contains shell commands executed in the workspace after clone but before the agent runs.
	// +optional
	Init []string `json:"init,omitempty"`

	// mcpServers defines MCP servers made available to the agent.
	// Supports remote (url) and stdio (command) transports.
	// +optional
	MCPServers []MCPServer `json:"mcpServers,omitempty"`
}

// MCPServer defines an MCP server to connect to the agent.
// The transport is inferred from the fields set:
//   - url → remote (streamable HTTP or SSE)
//   - command → stdio (agent spawns the process)
type MCPServer struct {
	// name is the identifier for this MCP server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// url is the endpoint for a remote MCP server (streamable HTTP or SSE).
	// Mutually exclusive with command.
	// +optional
	URL string `json:"url,omitempty"`

	// headers are HTTP headers sent with requests to a remote MCP server.
	// Only used when url is set.
	// +optional
	Headers []MCPHeader `json:"headers,omitempty"`

	// command is the program to spawn for a stdio-based MCP server.
	// Mutually exclusive with url.
	// +optional
	Command string `json:"command,omitempty"`

	// args are arguments passed to the command.
	// Only used when command is set.
	// +optional
	Args []string `json:"args,omitempty"`

	// env are environment variables set for the MCP server process (stdio)
	// or sent as context to a remote server.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
}

// MCPHeader defines an HTTP header for remote MCP server connections.
type MCPHeader struct {
	// name is the header name (e.g., "Authorization").
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// value is the header value. Mutually exclusive with valueFrom.
	// +optional
	Value string `json:"value,omitempty"`

	// valueFrom references a secret key for the header value.
	// Mutually exclusive with value.
	// +optional
	ValueFrom *corev1.SecretKeySelector `json:"valueFrom,omitempty"`
}

// SkillFile defines a file to inject into the agent workspace.
type SkillFile struct {
	// path is the relative path within the workspace where the file will be written.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`

	// content is the inline file content. Mutually exclusive with configMapRef.
	// +optional
	Content string `json:"content,omitempty"`

	// configMapRef references a key in a ConfigMap for the file content.
	// Mutually exclusive with content.
	// +optional
	ConfigMapRef *ConfigMapKeyRef `json:"configMapRef,omitempty"`
}

// ConfigMapKeyRef references a key within a ConfigMap.
type ConfigMapKeyRef struct {
	// name is the ConfigMap name.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// key is the key within the ConfigMap.
	// +kubebuilder:validation:Required
	Key string `json:"key"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=sk
// +kubebuilder:printcolumn:name="Description",type=string,JSONPath=`.spec.description`
// +kubebuilder:printcolumn:name="Files",type=integer,JSONPath=`.spec.files`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Skill is the Schema for the skills API.
// A Skill defines reusable configuration (files, env vars, init commands)
// that can be loaded into agent sandbox pods.
type Skill struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec SkillSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// SkillList contains a list of Skill.
type SkillList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Skill `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Skill{}, &SkillList{})
}
