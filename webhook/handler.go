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

package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	factoryv1alpha1 "github.com/kube-foundry/kube-foundry/api/v1alpha1"
)

// WebhookHandler handles HTTP requests to create SoftwareTask resources.
type WebhookHandler struct {
	client    client.Client
	namespace string
}

// CreateTaskRequest is the JSON body for creating a task via the REST API.
type CreateTaskRequest struct {
	Repo           string                      `json:"repo"`
	Branch         string                      `json:"branch,omitempty"`
	Task           string                      `json:"task"`
	SecretRef      string                      `json:"secretRef,omitempty"`
	GithubToken    string                      `json:"githubToken,omitempty"`
	Agent          string                      `json:"agent,omitempty"`
	CallbackURL    string                      `json:"callbackURL,omitempty"`
	GitAuthorName  string                      `json:"gitAuthorName,omitempty"`
	GitAuthorEmail string                      `json:"gitAuthorEmail,omitempty"`
	MaxRetries     *int32                      `json:"maxRetries,omitempty"`
	Skills         []string                    `json:"skills,omitempty"`
	MCPServers     []factoryv1alpha1.MCPServer `json:"mcpServers,omitempty"`
}

// CreateTaskResponse is returned after a task is created.
type CreateTaskResponse struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Message   string `json:"message"`
}

// CreateTask handles POST /api/v1/tasks.
func (h *WebhookHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Repo == "" || req.Task == "" {
		http.Error(w, "repo and task are required", http.StatusBadRequest)
		return
	}

	taskName := fmt.Sprintf("task-%d", time.Now().UnixMilli())

	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.SecretRef == "" {
		req.SecretRef = "factory-creds"
	}
	if req.Agent == "" {
		req.Agent = "claude-code"
	}

	task := &factoryv1alpha1.SoftwareTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskName,
			Namespace: h.namespace,
		},
		Spec: factoryv1alpha1.SoftwareTaskSpec{
			Repo:   req.Repo,
			Branch: req.Branch,
			Task:   req.Task,
			Agent:  req.Agent,
			Credentials: factoryv1alpha1.CredentialsSpec{
				SecretRef:   req.SecretRef,
				GithubToken: req.GithubToken,
			},
			CallbackURL:    req.CallbackURL,
			GitAuthorName:  req.GitAuthorName,
			GitAuthorEmail: req.GitAuthorEmail,
			MaxRetries:     req.MaxRetries,
			Skills:         req.Skills,
			MCPServers:     req.MCPServers,
		},
	}

	if err := h.client.Create(context.Background(), task); err != nil {
		http.Error(w, "Failed to create task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := CreateTaskResponse{
		Name:      taskName,
		Namespace: h.namespace,
		Message:   fmt.Sprintf("Task created. Watch with: kubectl get st %s -w", taskName),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// GitHubWebhookPayload is the relevant subset of a GitHub webhook event.
type GitHubWebhookPayload struct {
	Action string `json:"action"`
	Label  struct {
		Name string `json:"name"`
	} `json:"label"`
	Issue struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	} `json:"issue"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

// GitHubWebhook handles POST /webhooks/github.
func (h *WebhookHandler) GitHubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	webhookSecret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if webhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, sig, webhookSecret) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var payload GitHubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if payload.Action != "labeled" || payload.Label.Name != "factory:do" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ignored"))
		return
	}

	taskName := fmt.Sprintf("issue-%d-%d", payload.Issue.Number, time.Now().Unix())
	taskDescription := fmt.Sprintf("GitHub Issue #%d: %s\n\n%s",
		payload.Issue.Number, payload.Issue.Title, payload.Issue.Body)

	task := &factoryv1alpha1.SoftwareTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskName,
			Namespace: h.namespace,
			Labels: map[string]string{
				"factory.io/source":       "github-issue",
				"factory.io/issue-number": fmt.Sprintf("%d", payload.Issue.Number),
			},
		},
		Spec: factoryv1alpha1.SoftwareTaskSpec{
			Repo:   payload.Repository.CloneURL,
			Branch: "main",
			Task:   taskDescription,
			Agent:  "claude-code",
			Credentials: factoryv1alpha1.CredentialsSpec{
				SecretRef: "factory-creds",
			},
		},
	}

	if err := h.client.Create(context.Background(), task); err != nil {
		http.Error(w, "Failed to create task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"task":    taskName,
		"message": "Task created from GitHub issue",
	})
}

func verifySignature(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
