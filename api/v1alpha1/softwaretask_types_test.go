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
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestTaskPhaseConstants(t *testing.T) {
	tests := []struct {
		phase TaskPhase
		want  string
	}{
		{TaskPhasePending, "Pending"},
		{TaskPhaseRunning, "Running"},
		{TaskPhaseCompleted, "Completed"},
		{TaskPhaseFailed, "Failed"},
	}
	for _, tt := range tests {
		if string(tt.phase) != tt.want {
			t.Errorf("expected %q, got %q", tt.want, tt.phase)
		}
	}
}

func TestGroupVersionInfo(t *testing.T) {
	if GroupVersion.Group != "factory.factory.io" {
		t.Errorf("expected group 'factory.factory.io', got %q", GroupVersion.Group)
	}
	if GroupVersion.Version != "v1alpha1" {
		t.Errorf("expected version 'v1alpha1', got %q", GroupVersion.Version)
	}
}

func TestSchemeRegistration(t *testing.T) {
	s := runtime.NewScheme()
	if err := AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme failed: %v", err)
	}

	// Verify SoftwareTask is registered
	gvks, _, err := s.ObjectKinds(&SoftwareTask{})
	if err != nil {
		t.Fatalf("SoftwareTask not registered: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("expected at least one GVK for SoftwareTask")
	}
	if gvks[0].Kind != "SoftwareTask" {
		t.Errorf("expected kind 'SoftwareTask', got %q", gvks[0].Kind)
	}

	// Verify SoftwareTaskList is registered
	gvks, _, err = s.ObjectKinds(&SoftwareTaskList{})
	if err != nil {
		t.Fatalf("SoftwareTaskList not registered: %v", err)
	}
	if len(gvks) == 0 {
		t.Fatal("expected at least one GVK for SoftwareTaskList")
	}
}

func TestSoftwareTask_DeepCopy(t *testing.T) {
	maxRetries := int32(3)
	task := &SoftwareTask{
		Spec: SoftwareTaskSpec{
			Repo:   "https://github.com/test/repo",
			Branch: "main",
			Task:   "Add tests",
			Agent:  "claude-code",
			Credentials: CredentialsSpec{
				SecretRef: "my-secret",
			},
			MaxRetries: &maxRetries,
		},
		Status: SoftwareTaskStatus{
			Phase:   TaskPhaseRunning,
			PodName: "test-sandbox",
			Message: "Running",
		},
	}

	copied := task.DeepCopy()

	// Verify it's a distinct object
	if copied == task {
		t.Fatal("DeepCopy returned same pointer")
	}

	// Verify values match
	if copied.Spec.Repo != task.Spec.Repo {
		t.Errorf("Repo mismatch: %q vs %q", copied.Spec.Repo, task.Spec.Repo)
	}
	if copied.Status.Phase != task.Status.Phase {
		t.Errorf("Phase mismatch: %q vs %q", copied.Status.Phase, task.Status.Phase)
	}

	// Verify MaxRetries is a separate pointer
	if copied.Spec.MaxRetries == task.Spec.MaxRetries {
		t.Error("MaxRetries should be a distinct pointer after DeepCopy")
	}
	if *copied.Spec.MaxRetries != *task.Spec.MaxRetries {
		t.Errorf("MaxRetries value mismatch: %d vs %d", *copied.Spec.MaxRetries, *task.Spec.MaxRetries)
	}

	// Mutating the copy shouldn't affect the original
	*copied.Spec.MaxRetries = 5
	if *task.Spec.MaxRetries != 3 {
		t.Error("modifying copy affected original")
	}
}
