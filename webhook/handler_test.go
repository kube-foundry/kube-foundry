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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	factoryv1alpha1 "github.com/kube-foundry/kube-foundry/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = factoryv1alpha1.AddToScheme(s)
	return s
}

func newTestHandler() *WebhookHandler {
	s := newTestScheme()
	fc := fake.NewClientBuilder().WithScheme(s).Build()
	return &WebhookHandler{
		client:    fc,
		namespace: "default",
	}
}

// --- CreateTask tests ---

func TestCreateTask_ValidRequest(t *testing.T) {
	h := newTestHandler()
	body := `{"repo":"https://github.com/test/repo","task":"Add feature X"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp CreateTaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", resp.Namespace)
	}
	if resp.Name == "" {
		t.Error("expected non-empty task name")
	}
}

func TestCreateTask_WithDefaults(t *testing.T) {
	h := newTestHandler()
	body := `{"repo":"https://github.com/test/repo","task":"Add feature"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
}

func TestCreateTask_WithAllFields(t *testing.T) {
	h := newTestHandler()
	body := `{"repo":"https://github.com/test/repo","task":"Add feature",` +
		`"branch":"dev","secretRef":"my-secret","agent":"claude-code"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
}

func TestCreateTask_MissingRepo(t *testing.T) {
	h := newTestHandler()
	body := `{"task":"Add feature X"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateTask_MissingTask(t *testing.T) {
	h := newTestHandler()
	body := `{"repo":"https://github.com/test/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateTask_InvalidJSON(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCreateTask_WithSkillsAndMCP(t *testing.T) {
	h := newTestHandler()
	body := `{
		"repo": "https://github.com/test/repo",
		"task": "Add feature",
		"skills": ["go-expert", "testing"],
		"mcpServers": [{"name": "internal", "url": "https://mcp.example.com/sse"}],
		"maxRetries": 3
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp CreateTaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify the created SoftwareTask has the fields set
	var taskList factoryv1alpha1.SoftwareTaskList
	if err := h.client.List(req.Context(), &taskList); err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(taskList.Items) != 1 {
		t.Fatalf("expected 1 task, got %d", len(taskList.Items))
	}
	created := taskList.Items[0]

	if len(created.Spec.Skills) != 2 || created.Spec.Skills[0] != "go-expert" || created.Spec.Skills[1] != "testing" {
		t.Errorf("expected skills [go-expert, testing], got %v", created.Spec.Skills)
	}
	if len(created.Spec.MCPServers) != 1 || created.Spec.MCPServers[0].Name != "internal" {
		t.Errorf("expected 1 MCP server named 'internal', got %v", created.Spec.MCPServers)
	}
	if created.Spec.MaxRetries == nil || *created.Spec.MaxRetries != 3 {
		t.Errorf("expected maxRetries=3, got %v", created.Spec.MaxRetries)
	}
}

func TestCreateTask_WrongMethod(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()

	h.CreateTask(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// --- GitHubWebhook tests ---

func makeGitHubPayload(action, label string) string {
	p := GitHubWebhookPayload{
		Action: action,
	}
	p.Label.Name = label
	p.Issue.Number = 42
	p.Issue.Title = "Test issue"
	p.Issue.Body = "Test body"
	p.Repository.FullName = "test/repo"
	p.Repository.CloneURL = "https://github.com/test/repo.git"

	b, _ := json.Marshal(p)
	return string(b)
}

func computeSignature(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestGitHubWebhook_ValidLabeledEvent(t *testing.T) {
	h := newTestHandler()
	body := makeGitHubPayload("labeled", "factory:do")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["task"] == "" {
		t.Error("expected non-empty task name")
	}
	if resp["message"] == "" {
		t.Error("expected non-empty message")
	}
}

func TestGitHubWebhook_WrongAction(t *testing.T) {
	h := newTestHandler()
	body := makeGitHubPayload("opened", "factory:do")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
	if w.Body.String() != "ignored" {
		t.Errorf("expected 'ignored', got %q", w.Body.String())
	}
}

func TestGitHubWebhook_WrongLabel(t *testing.T) {
	h := newTestHandler()
	body := makeGitHubPayload("labeled", "bug")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGitHubWebhook_WrongMethod(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/webhooks/github", nil)
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestGitHubWebhook_InvalidJSON(t *testing.T) {
	h := newTestHandler()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader("{bad"))
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGitHubWebhook_ValidSignature(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")

	h := newTestHandler()
	body := makeGitHubPayload("labeled", "factory:do")
	sig := computeSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected %d, got %d: %s", http.StatusCreated, w.Code, w.Body.String())
	}
}

func TestGitHubWebhook_InvalidSignature(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")

	h := newTestHandler()
	body := makeGitHubPayload("labeled", "factory:do")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestGitHubWebhook_MissingSignatureWhenSecretSet(t *testing.T) {
	t.Setenv("GITHUB_WEBHOOK_SECRET", "test-secret")

	h := newTestHandler()
	body := makeGitHubPayload("labeled", "factory:do")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(body))
	// No signature header
	w := httptest.NewRecorder()

	h.GitHubWebhook(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// --- verifySignature tests ---

func TestVerifySignature_Valid(t *testing.T) {
	body := []byte("hello world")
	secret := "my-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifySignature(body, sig, secret) {
		t.Error("expected valid signature to pass")
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	if verifySignature([]byte("hello"), "sha256=bad", "secret") {
		t.Error("expected invalid signature to fail")
	}
}

func TestVerifySignature_EmptyBody(t *testing.T) {
	body := []byte("")
	secret := "my-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifySignature(body, sig, secret) {
		t.Error("expected valid signature for empty body to pass")
	}
}

func TestVerifySignature_WrongSecret(t *testing.T) {
	body := []byte("hello")
	mac := hmac.New(sha256.New, []byte("secret1"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if verifySignature(body, sig, "secret2") {
		t.Error("expected signature with wrong secret to fail")
	}
}
